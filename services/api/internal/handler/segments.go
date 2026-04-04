package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

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
		IsAdmin:        isAdmin,
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	create, ok := httputil.DecodeJSON[model.SegmentCreate](w, r)
	if !ok {
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

	if create.Visibility == "" {
		create.Visibility = "public"
	} else if create.Visibility != "public" && create.Visibility != "private" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "visibility must be 'public' or 'private'", r.URL.Path)
		return
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
	// For private segments, backfill only runs against creator's own activities.
	// GetSpatialCandidateSegments enforces: s.visibility = 'public' OR s.created_by = userID
	segID := segment.ID
	shutdown.Go("segment-backfill-"+segID, func(ctx context.Context) {
		ids, err := h.DB.ListActivityIDsForSegmentBackfill(ctx, userID, segID)
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
			MatchActivitySegments(ctx, h.DB, actID, userID,
				act.SportType, subType, act.GearID, act.StartedAt)
		}
		slog.Info("segment backfill: done", "segment_id", segID)
	})
}

func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	segment, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin)
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	// Load existing segment: pass userID so own private segments are visible.
	// (Pre-existing bug fix: was GetSegment(ctx, id, "") which only returns public segments.)
	existing, err := h.DB.GetSegment(r.Context(), id, userID, false)
	if err != nil {
		slog.Error("update segment: get segment failed", "id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load segment", r.URL.Path)
		return
	}
	if existing == nil || existing.CreatedBy != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	update, ok := httputil.DecodeJSON[model.SegmentUpdate](w, r)
	if !ok {
		return
	}

	// Validate visibility value (clean 400 instead of DB CHECK constraint 500).
	if update.Visibility != nil && *update.Visibility != "public" && *update.Visibility != "private" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "visibility must be 'public' or 'private'", r.URL.Path)
		return
	}

	// Block private→public via UpdateSegment — must use POST /segments/{id}/publish endpoint
	// which triggers a full recompute for all users' activities.
	if update.Visibility != nil && *update.Visibility == "public" && existing.Visibility == "private" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "use POST /segments/{id}/publish to publish a private segment", r.URL.Path)
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

	// If switching public→private: recompute counts so effort_count/athlete_count
	// no longer include other users' efforts. Track as background_job (ActiveJobsIndicator).
	if update.Visibility != nil && *update.Visibility == "private" && existing.Visibility == "public" {
		entityName := existing.Name
		jID, _ := h.DB.InsertBackgroundJob(r.Context(), userID, "segment_recompute", &id, &entityName, 1)
		shutdown.Go("segment-to-private-"+id, func(ctx context.Context) {
			failed := false
			if err := h.DB.UpdateSegmentEffortRanks(ctx, id); err != nil {
				slog.Warn("update segment: recompute after public→private failed", "segment_id", id, "error", err)
				failed = true
			}
			if jID != "" {
				h.DB.CompleteBackgroundJob(ctx, jID, failed, "")
			}
		})
	}

	httputil.WriteJSON(w, http.StatusOK, segment)
}

// DeleteSegments bulk-deletes multiple segments owned by the authenticated user.
//
// DELETE /api/v1/segments
// Body: {"ids": ["uuid1", "uuid2", ...]}
// OSM-imported lift segments are silently skipped (not counted in the response).
func (h *Handler) DeleteSegments(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	body, ok := httputil.DecodeJSON[struct {
		IDs []string `json:"ids"`
	}](w, r)
	if !ok {
		return
	}
	if len(body.IDs) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "ids array required", r.URL.Path)
		return
	}
	if len(body.IDs) > 500 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "max 500 ids per request", r.URL.Path)
		return
	}

	count, err := h.DB.DeleteSegments(r.Context(), body.IDs, userID)
	if err != nil {
		slog.Error("bulk delete segments failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete segments", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": count})
}

func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	if err := h.DB.DeleteSegment(r.Context(), userID, id, isAdmin); err != nil {
		if errors.Is(err, database.ErrSegmentOSMLift) {
			httputil.WriteError(w, http.StatusForbidden, "Forbidden", "OSM-imported lift segments cannot be deleted", r.URL.Path)
			return
		}
		// Return 404 for both not-found and not-authorized — prevents segment existence oracle.
		if errors.Is(err, database.ErrSegmentNotFound) || errors.Is(err, database.ErrSegmentNotAuthorized) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	// Verify segment is visible to the requesting user (prevents private segment data leak via UUID).
	seg, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin)
	if err != nil {
		slog.Error("get segment leaderboard: get segment failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	sportType := r.URL.Query().Get("sport_type")
	gearID := r.URL.Query().Get("gear_id")

	var isReverse *bool
	if v := r.URL.Query().Get("is_reverse"); v != "" {
		b := v == "true"
		isReverse = &b
	}

	// "only_mine=true" shows the current user's own efforts, including from private activities.
	var onlyUserID string
	if r.URL.Query().Get("only_mine") == "true" {
		onlyUserID = userID
	}

	efforts, err := h.DB.GetSegmentLeaderboard(r.Context(), id, sportType, isReverse, gearID, onlyUserID)
	if err != nil {
		slog.Error("get segment leaderboard failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get leaderboard", r.URL.Path)
		return
	}

	// Strip health-sensitive fields from other users' entries (DSGVO Art. 9:
	// avg_heart_rate and avg_power_w are personal health data).
	for i := range efforts {
		if efforts[i].UserID != userID {
			efforts[i].AvgPowerW = nil
			efforts[i].AvgHeartRate = nil
			efforts[i].AvgSpeedMs = nil
		}
	}

	httputil.WriteJSON(w, http.StatusOK, efforts)
}

func (h *Handler) GetActivitySegmentEfforts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), id, userID, isAdmin)
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

	shutdown.Go("segment-recompute-"+id, func(ctx context.Context) {
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
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "recompute started",
		"job_id": jobID,
	})
}

// PublishSegment transitions a private segment to public and triggers a full recompute
// so all users' activities are matched. Uses background_job tracking (visible in ActiveJobsIndicator).
func (h *Handler) PublishSegment(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")

	// Get segment to verify ownership and retrieve name for job display.
	seg, err := h.DB.GetSegment(r.Context(), id, userID, false)
	if err != nil {
		slog.Error("publish segment: get segment failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil || seg.CreatedBy != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found or not owned", r.URL.Path)
		return
	}
	if seg.Visibility != "private" {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "already_public"})
		return
	}

	// List all nearby activities (all users) before publishing, so we know the total.
	ids, err := h.DB.ListAllActivityIDsForSegmentBackfill(r.Context(), id)
	if err != nil {
		slog.Error("publish segment: list activities failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}

	// Set visibility = 'public'. After this point the segment is visible to all users.
	published, err := h.DB.PublishSegment(r.Context(), id, userID)
	if err != nil {
		slog.Error("publish segment failed", "segment_id", id, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to publish segment", r.URL.Path)
		return
	}
	if !published {
		// Race: another request already published it between our check and this call.
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "already_public"})
		return
	}

	// Cancel any in-progress recompute jobs for this segment to prevent races (A8).
	// Must run BEFORE InsertBackgroundJob — otherwise it cancels the job we just created.
	h.DB.CancelRunningJobsForEntity(r.Context(), userID, id)

	// Track the recompute as a background job (visible in ActiveJobsIndicator).
	entityName := seg.Name
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "segment_publish", &id, &entityName, int64(len(ids)))
	if err != nil {
		slog.Warn("publish segment: failed to insert job", "segment_id", id, "error", err)
		jobID = ""
	}

	shutdown.Go("segment-publish-"+id, func(ctx context.Context) {
		failed := false

		if err := h.DB.DeleteAllSegmentEffortsBySegment(ctx, id); err != nil {
			slog.Error("publish recompute: delete efforts failed", "segment_id", id, "error", err)
			failed = true
		}

		if !failed {
			for i, actID := range ids {
				act, err := h.DB.GetActivity(ctx, actID)
				if err != nil || act == nil {
					continue
				}
				subType := ""
				if act.SubType != nil {
					subType = *act.SubType
				}
				MatchActivitySegments(ctx, h.DB, actID, act.UserID, act.SportType, subType, act.GearID, act.StartedAt)

				if jobID != "" && (i%10 == 0 || i == len(ids)-1) {
					if err := h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(i+1)); err != nil {
						slog.Warn("publish recompute: update progress failed", "job_id", jobID, "error", err)
					}
				}
			}
			slog.Info("segment publish recompute: done", "segment_id", id, "activity_count", len(ids))
		}

		// Final rank/count update — ensures correct values even if zero activities matched.
		if err := h.DB.UpdateSegmentEffortRanks(ctx, id); err != nil {
			slog.Warn("publish recompute: final rank update failed", "segment_id", id, "error", err)
		}

		if jobID != "" {
			if err := h.DB.CompleteBackgroundJob(ctx, jobID, failed, ""); err != nil {
				slog.Warn("publish recompute: complete job failed", "job_id", jobID, "error", err)
			}
		}
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "publish started",
		"job_id": jobID,
	})
}

func (h *Handler) GetActivityDetectedLaps(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin)
	if err != nil {
		slog.Error("segment access check failed", "handler", "GetSegmentFavoriteStatus", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

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
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin)
	if err != nil {
		slog.Error("segment access check failed", "handler", "AddSegmentFavorite", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	if err := h.DB.AddSegmentFavorite(r.Context(), userID, segmentID); err != nil {
		slog.Error("add favorite failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add favorite", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// RemoveSegmentFavorite removes a segment from the user's favorites.
func (h *Handler) RemoveSegmentFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin)
	if err != nil {
		slog.Error("segment access check failed", "handler", "RemoveSegmentFavorite", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

	if err := h.DB.RemoveSegmentFavorite(r.Context(), userID, segmentID); err != nil {
		slog.Error("remove favorite failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to remove favorite", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// GetSegmentPerformances retrieves all efforts on a segment from the user's activities.
func (h *Handler) GetSegmentPerformances(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	segmentID := chi.URLParam(r, "id")
	isAdmin, _ := r.Context().Value(middleware.IsAdminKey).(bool)

	seg, err := h.DB.GetSegment(r.Context(), segmentID, userID, isAdmin)
	if err != nil {
		slog.Error("segment access check failed", "handler", "GetSegmentPerformances", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment", r.URL.Path)
		return
	}
	if seg == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "segment not found", r.URL.Path)
		return
	}

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

// GetSegmentPRs returns all segment efforts where the authenticated user achieved personal rank 1–3.
func (h *Handler) GetSegmentPRs(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	efforts, err := h.DB.GetUserSegmentPRs(r.Context(), userID)
	if err != nil {
		slog.Error("get segment prs failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get segment PRs", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"efforts": efforts,
		"count":   len(efforts),
	})
}
