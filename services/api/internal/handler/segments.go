package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	sportGroup := r.URL.Query().Get("sport_group")
	segmentType := r.URL.Query().Get("segment_type")
	name := r.URL.Query().Get("name")
	bbox := r.URL.Query().Get("bbox")

	segments, err := h.DB.ListSegments(r.Context(), userID, sportGroup, segmentType, name, bbox)
	if err != nil {
		slog.Error("list segments failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list segments", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, segments)
}

func (h *Handler) CreateSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var create model.SegmentCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.GeoJSONTrack == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "geojson_track is required", r.URL.Path)
		return
	}
	if create.SportGroup == "" {
		create.SportGroup = "generic"
	}
	if create.SegmentType == "" {
		create.SegmentType = "standard"
	}

	segment, err := h.DB.CreateSegment(r.Context(), userID, &create)
	if err != nil {
		if strings.Contains(err.Error(), "lap segment must be a closed loop") {
			writeError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error(), r.URL.Path)
			return
		}
		slog.Error("create segment failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create segment", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusCreated, segment)

	// Backfill: match new segment against all existing activities asynchronously.
	go func(segID, uID string) {
		ctx := context.Background()
		ids, err := h.DB.ListActivityIDsForSegmentBackfill(ctx, uID, segID)
		if err != nil {
			slog.Error("segment backfill: list activities failed",
				"segment_id", segID, "error", err)
			return
		}
		slog.Info("segment backfill: starting",
			"segment_id", segID, "activity_count", len(ids))
		for _, actID := range ids {
			act, err := h.DB.GetActivity(ctx, actID)
			if err != nil || act == nil {
				continue
			}
			subType := ""
			if act.SubType != nil {
				subType = *act.SubType
			}
			MatchActivitySegments(ctx, h.DB, actID, uID,
				act.SportType, subType, act.GearID)
		}
		slog.Info("segment backfill: done", "segment_id", segID)
	}(segment.ID, userID)
}

func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	segment, err := h.DB.GetSegment(r.Context(), id)
	if err != nil {
		slog.Error("get segment failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if segment == nil {
		writeError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, segment)
}

func (h *Handler) UpdateSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	// Verify ownership before mutating.
	existing, err := h.DB.GetSegment(r.Context(), id)
	if err != nil {
		slog.Error("update segment: get segment failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load segment", r.URL.Path)
		return
	}
	if existing == nil || existing.CreatedBy != userID {
		writeError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	var update model.SegmentUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	segment, err := h.DB.UpdateSegment(r.Context(), id, &update)
	if err != nil {
		slog.Error("update segment failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update segment", r.URL.Path)
		return
	}
	if segment == nil {
		writeError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, segment)
}

func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteSegment(r.Context(), userID, id); err != nil {
		if err.Error() == "segment not found or not owned by user" {
			writeError(w, http.StatusNotFound, "Not Found", "segment not found or not owned", r.URL.Path)
			return
		}
		slog.Error("delete segment failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete segment", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetSegmentLeaderboard(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sportType := r.URL.Query().Get("sport_type")
	gearID := r.URL.Query().Get("gear_id")

	var isReverse *bool
	if v := r.URL.Query().Get("is_reverse"); v != "" {
		b := v == "true"
		isReverse = &b
	}

	efforts, err := h.DB.GetSegmentLeaderboard(r.Context(), id, sportType, isReverse, gearID)
	if err != nil {
		slog.Error("get segment leaderboard failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get leaderboard", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, efforts)
}

func (h *Handler) GetActivitySegmentEfforts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get activity segment efforts: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	efforts, err := h.DB.GetActivitySegmentEfforts(r.Context(), id)
	if err != nil {
		slog.Error("get activity segment efforts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment efforts", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, efforts)
}

// RecomputeSegment re-matches all of the user's GPS activities against a single segment.
// Useful after editing a segment or when matching failed due to a bug.
func (h *Handler) RecomputeSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	seg, err := h.DB.GetSegment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		writeError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	go func() {
		ctx := context.Background()
		ids, err := h.DB.ListActivityIDsForSegmentBackfill(ctx, userID, id)
		if err != nil {
			slog.Error("segment recompute: list activities failed", "segment_id", id, "error", err)
			return
		}
		// Delete existing efforts for this segment by this user before re-matching
		if err := h.DB.DeleteSegmentEffortsBySegment(ctx, id, userID); err != nil {
			slog.Error("segment recompute: delete existing efforts failed", "segment_id", id, "error", err)
			return
		}
		slog.Info("segment recompute: starting", "segment_id", id, "activity_count", len(ids))
		for _, actID := range ids {
			act, err := h.DB.GetActivity(ctx, actID)
			if err != nil || act == nil {
				continue
			}
			subType := ""
			if act.SubType != nil {
				subType = *act.SubType
			}
			MatchActivitySegments(ctx, h.DB, actID, userID, act.SportType, subType, act.GearID)
		}
		slog.Info("segment recompute: done", "segment_id", id)
	}()

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "recompute started"})
}

func (h *Handler) GetActivityDetectedLaps(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get detected laps: get activity failed", "id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	laps, err := h.DB.GetDetectedLaps(r.Context(), id)
	if err != nil {
		slog.Error("get detected laps failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get detected laps", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, laps)
}
