package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if l == "-1" {
			limit = -1
		} else if parsed, err := strconv.Atoi(l); err == nil && parsed >= 0 {
			limit = parsed
		}
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name = r.URL.Query().Get("search")
	}

	params := model.ListSegmentsParams{
		UserID:         userID,
		SportGroup:     r.URL.Query().Get("sport_group"),
		SegmentType:    r.URL.Query().Get("segment_type"),
		SegmentTypes:   r.URL.Query().Get("segment_types"),
		Name:           name,
		BBox:           r.URL.Query().Get("bbox"),
		OnlyRidden:     r.URL.Query().Get("only_ridden") == "true",
		IncludeGeoJSON: r.URL.Query().Get("include_geojson") == "true",
		Cursor:         r.URL.Query().Get("cursor"),
		Limit:          limit,
	}

	result, err := h.DB.ListSegments(r.Context(), params)
	if err != nil {
		slog.Error("list segments failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list segments", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) CreateSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var create model.SegmentCreate
	if err := json.NewDecoder(r.Body).Decode(&create); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if create.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if create.GeoJSONTrack == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "geojson_track is required", r.URL.Path)
		return
	}
	if create.SportGroup == "" {
		create.SportGroup = "generic"
	}
	if create.SegmentType == "" {
		create.SegmentType = "standard"
	}
	if create.TerrainType == "" {
		create.TerrainType = "standard"
	}
	if create.ClimbType == "" {
		create.ClimbType = "standard"
	}
	// Lift segments always get bergfahrt as default climb_type
	if create.TerrainType == "lift" && create.ClimbType == "standard" {
		create.ClimbType = "bergfahrt"
	}

	segment, err := h.DB.CreateSegment(r.Context(), userID, &create)
	if err != nil {
		if strings.Contains(err.Error(), "lap segment must be a closed loop") {
			httputil.WriteError(w, http.StatusUnprocessableEntity, "Validation Error", err.Error(), r.URL.Path)
			return
		}
		slog.Error("create segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create segment", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, segment)

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
				act.SportType, subType, act.GearID, act.StartedAt)
		}
		slog.Info("segment backfill: done", "segment_id", segID)
	}(segment.ID, userID)
}

func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID := middleware.GetUserID(r.Context())

	segment, err := h.DB.GetSegment(r.Context(), id, userID)
	if err != nil {
		slog.Error("get segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if segment == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, segment)
}

func (h *Handler) UpdateSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	// Verify ownership before mutating.
	existing, err := h.DB.GetSegment(r.Context(), id, "")
	if err != nil {
		slog.Error("update segment: get segment failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load segment", r.URL.Path)
		return
	}
	if existing == nil || existing.CreatedBy != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	var update model.SegmentUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	segment, err := h.DB.UpdateSegment(r.Context(), id, userID, &update)
	if err != nil {
		slog.Error("update segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update segment", r.URL.Path)
		return
	}
	if segment == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, segment)
}

func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	if err := h.DB.DeleteSegment(r.Context(), userID, id); err != nil {
		if strings.Contains(err.Error(), "cannot be deleted") {
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", err.Error(), r.URL.Path)
			return
		}
		if err.Error() == "segment not found or not owned by user" {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found or not owned", r.URL.Path)
			return
		}
		slog.Error("delete segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete segment", r.URL.Path)
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
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get leaderboard", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, efforts)
}

func (h *Handler) GetActivitySegmentEfforts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get activity segment efforts: get activity failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	efforts, err := h.DB.GetActivitySegmentEfforts(r.Context(), id)
	if err != nil {
		slog.Error("get activity segment efforts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment efforts", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, efforts)
}

// RecomputeSegment re-matches all of the user's GPS activities against a single segment.
// Useful after editing a segment or when matching failed due to a bug.
func (h *Handler) RecomputeSegment(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	seg, err := h.DB.GetSegment(r.Context(), id, userID)
	if err != nil {
		slog.Error("segment recompute: get segment failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	// Aktivitäts-IDs vorab ermitteln (brauchen wir für progress_total).
	ids, err := h.DB.ListActivityIDsForSegmentBackfill(r.Context(), userID, id)
	if err != nil {
		slog.Error("segment recompute: list activities failed", "segment_id", id, "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	// Vorherige laufende Jobs für dieses Segment abbrechen.
	if err := h.DB.CancelRunningJobsForEntity(r.Context(), userID, id); err != nil {
		slog.Warn("segment recompute: cancel previous jobs failed", "segment_id", id, "error", err)
	}

	// Job in DB anlegen.
	entityName := seg.Name
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "segment_recompute", &id, &entityName, int64(len(ids)))
	if err != nil {
		slog.Warn("segment recompute: failed to insert job", "segment_id", id, "error", err)
		jobID = ""
	}

	go func() {
		ctx := context.Background()
		failed := false

		if err := h.DB.DeleteSegmentEffortsBySegment(ctx, id, userID); err != nil {
			slog.Error("segment recompute: delete existing efforts failed", "segment_id", id, "error", err)
			failed = true
		}

		if !failed {
			slog.Info("segment recompute: starting", "segment_id", id, "activity_count", len(ids))
			for i, actID := range ids {
				act, err := h.DB.GetActivity(ctx, actID)
				if err != nil || act == nil {
					continue
				}
				subType := ""
				if act.SubType != nil {
					subType = *act.SubType
				}
				MatchActivitySegments(ctx, h.DB, actID, userID, act.SportType, subType, act.GearID, act.StartedAt)

				// Fortschritt alle 10 Aktivitäten oder am Ende aktualisieren.
				if jobID != "" && (i%10 == 0 || i == len(ids)-1) {
					if err := h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1)); err != nil {
						slog.Warn("segment recompute: update progress failed", "job_id", jobID, "error", err)
					}
				}
			}
			slog.Info("segment recompute: done", "segment_id", id)
		}

		if jobID != "" {
			if err := h.DB.CompleteBackgroundJob(ctx, jobID, failed, ""); err != nil {
				slog.Warn("segment recompute: complete job failed", "job_id", jobID, "error", err)
			}
		}
	}()

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "recompute started",
		"job_id": jobID,
	})
}

func (h *Handler) GetActivityDetectedLaps(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	id := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), id)
	if err != nil {
		slog.Error("get detected laps: get activity failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	laps, err := h.DB.GetDetectedLaps(r.Context(), id)
	if err != nil {
		slog.Error("get detected laps failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get detected laps", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, laps)
}

// GetSegmentFavoriteStatus checks if a segment is favorited by the user.
func (h *Handler) GetSegmentFavoriteStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	segmentID := chi.URLParam(r, "id")

	isFavorite, err := h.DB.IsSegmentFavoriteByUser(r.Context(), userID, segmentID)
	if err != nil {
		slog.Error("check favorite status failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to check favorite status", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"favorite": isFavorite})
}

// AddSegmentFavorite adds a segment to the user's favorites.
func (h *Handler) AddSegmentFavorite(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	segmentID := chi.URLParam(r, "id")

	if err := h.DB.AddSegmentFavorite(r.Context(), userID, segmentID); err != nil {
		slog.Error("add favorite failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add favorite", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// RemoveSegmentFavorite removes a segment from the user's favorites.
func (h *Handler) RemoveSegmentFavorite(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	segmentID := chi.URLParam(r, "id")

	if err := h.DB.RemoveSegmentFavorite(r.Context(), userID, segmentID); err != nil {
		slog.Error("remove favorite failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to remove favorite", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// GetSegmentPerformances retrieves all efforts on a segment from the user's activities.
func (h *Handler) GetSegmentPerformances(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}
	segmentID := chi.URLParam(r, "id")

	var isReverse *bool
	if v := r.URL.Query().Get("is_reverse"); v != "" {
		b := v == "true"
		isReverse = &b
	}

	performances, err := h.DB.GetSegmentPerformances(r.Context(), segmentID, userID, isReverse)
	if err != nil {
		slog.Error("get segment performances failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch performances", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"performances": performances,
		"count":        len(performances),
	})
}
