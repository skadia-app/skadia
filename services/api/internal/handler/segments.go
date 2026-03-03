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
	id := chi.URLParam(r, "id")

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
	id := chi.URLParam(r, "id")

	efforts, err := h.DB.GetActivitySegmentEfforts(r.Context(), id)
	if err != nil {
		slog.Error("get activity segment efforts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment efforts", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, efforts)
}

func (h *Handler) GetActivityDetectedLaps(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	laps, err := h.DB.GetDetectedLaps(r.Context(), id)
	if err != nil {
		slog.Error("get detected laps failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get detected laps", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, laps)
}
