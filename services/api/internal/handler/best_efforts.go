package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// GetActivityBestEfforts returns all best effort records for a single activity,
// each annotated with the user's all-time rank (top-3 only), plus near-miss
// entries for efforts that almost beat the user's current personal record.
//
// Response shape:
//
//	{
//	  "best_efforts": [...],
//	  "near_misses":  [...]
//	}
//
// GET /api/v1/activities/{id}/best-efforts
func (h *Handler) GetActivityBestEfforts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity id is required", r.URL.Path)
		return
	}

	bestEfforts, err := h.DB.GetBestEffortsForActivity(r.Context(), activityID, userID)
	if err != nil {
		slog.Error("get best efforts for activity failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get best efforts", r.URL.Path)
		return
	}

	nearMisses, err := h.DB.GetNearMisses(r.Context(), activityID, userID)
	if err != nil {
		slog.Error("get near misses failed", "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get near misses", r.URL.Path)
		return
	}

	// Merge percentiles into each best effort entry. A missing key means the
	// category has fewer than 5 eligible efforts; Percentile stays nil.
	if len(bestEfforts) > 0 {
		percentiles, err := h.DB.GetEffortPercentiles(r.Context(), activityID, userID)
		if err != nil {
			// Non-fatal: percentiles are informational; log and continue.
			slog.Warn("get effort percentiles failed", "activity_id", activityID, "error", err)
		} else {
			for i := range bestEfforts {
				key := bestEfforts[i].Category + ":" + bestEfforts[i].SportGroup
				if pct, ok := percentiles[key]; ok {
					p := pct
					bestEfforts[i].Percentile = &p
				}
			}
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"best_efforts": bestEfforts,
		"near_misses":  nearMisses,
	})
}

// GetBestEffortRecords returns the top-N personal records for the authenticated user,
// optionally filtered by sport group and/or year.
//
// Query params:
//   - sport_group: e.g. "running", "cycling" (optional)
//   - year: e.g. "2026" — filters to [Jan 1, Jan 1 next year) (optional)
//
// GET /api/v1/personal-records (best-efforts flavour, distinct from analytics personal records)
func (h *Handler) GetBestEffortRecords(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	sportGroup := r.URL.Query().Get("sport_group")

	var achievedAfter, achievedBefore *time.Time
	if yearStr := r.URL.Query().Get("year"); yearStr != "" {
		year, err := strconv.Atoi(yearStr)
		if err != nil || year < 1900 || year > 2100 {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid year parameter", r.URL.Path)
			return
		}
		after := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
		before := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.UTC)
		achievedAfter = &after
		achievedBefore = &before
	}

	const defaultTopN = 3
	results, err := h.DB.GetBestEffortPersonalRecords(r.Context(), userID, sportGroup, defaultTopN, achievedAfter, achievedBefore)
	if err != nil {
		slog.Error("get best effort personal records failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get personal records", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, results)
}

// GetBestEffortExclusions returns all exclusion records for the authenticated user,
// newest first.
//
// GET /api/v1/best-effort-exclusions
func (h *Handler) GetBestEffortExclusions(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	exclusions, err := h.DB.GetBestEffortExclusions(r.Context(), userID)
	if err != nil {
		slog.Error("get best effort exclusions failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get exclusions", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, exclusions)
}

// AddBestEffortExclusion marks an activity's best effort (or all efforts via "__all__")
// as excluded from ranking for the authenticated user.
//
// Body: {"activity_id": "...", "category": "dist_5km"}
//
// POST /api/v1/best-effort-exclusions
func (h *Handler) AddBestEffortExclusion(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	body, ok := httputil.DecodeJSON[struct {
		ActivityID string `json:"activity_id"`
		Category   string `json:"category"`
	}](w, r)
	if !ok {
		return
	}
	if body.ActivityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activity_id is required", r.URL.Path)
		return
	}
	if body.Category == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "category is required", r.URL.Path)
		return
	}

	if err := h.DB.UpsertBestEffortExclusion(r.Context(), userID, body.ActivityID, body.Category); err != nil {
		slog.Error("upsert best effort exclusion failed", "user_id", userID, "activity_id", body.ActivityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add exclusion", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveBestEffortExclusion removes an exclusion so the effort is ranked again.
//
// DELETE /api/v1/best-effort-exclusions/{activityID}/{category}
func (h *Handler) RemoveBestEffortExclusion(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	activityID := chi.URLParam(r, "activityID")
	category := chi.URLParam(r, "category")
	if activityID == "" || category == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "activityID and category are required", r.URL.Path)
		return
	}

	if err := h.DB.DeleteBestEffortExclusion(r.Context(), userID, activityID, category); err != nil {
		slog.Error("delete best effort exclusion failed", "user_id", userID, "activity_id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to remove exclusion", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetPRHistory returns the full chronological progression of best efforts for
// a (user, category, sport_group) triple — up to 200 entries — each annotated
// with its all-time performance rank. Intended for the PR Progression Timeline UI.
//
// Query params:
//   - category:    e.g. "dist_5km"  (required)
//   - sport_group: e.g. "running"   (required)
//
// GET /api/v1/personal-records/history
func (h *Handler) GetPRHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	category := r.URL.Query().Get("category")
	sportGroup := r.URL.Query().Get("sport_group")
	if category == "" || sportGroup == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "category and sport_group required", r.URL.Path)
		return
	}
	entries, err := h.DB.GetPRHistory(r.Context(), userID, category, sportGroup)
	if err != nil {
		slog.Error("get PR history failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to fetch", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, entries)
}

// RecalcBestEfforts starts a background job that deletes all best effort records
// for the user and recomputes them from scratch in chronological order.
// Uses InsertBackgroundJobGuarded to prevent duplicate runs.
//
// POST /api/v1/best-efforts/recalc
func (h *Handler) RecalcBestEfforts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if h.BestEfforts == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Service Unavailable", "best efforts service not configured", r.URL.Path)
		return
	}

	entityName := "Bestleistungen"
	jobID, created, err := h.DB.InsertBackgroundJobGuarded(r.Context(), userID,
		"recalc_best_efforts", nil, &entityName, 0)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Job creation failed", err.Error(), r.URL.Path)
		return
	}
	if !created {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	svc := h.BestEfforts
	shutdown.Go(jobID, func(ctx context.Context) {
		err := svc.RecalcAll(ctx, userID, func(done, total int) {
			_ = h.DB.UpdateBackgroundJobProgress(ctx, jobID, int64(done))
			if done == 0 && total > 0 {
				_ = h.DB.UpdateBackgroundJobTotal(ctx, jobID, int64(total))
			}
		})
		if err != nil {
			slog.Error("RecalcBestEfforts failed", "user_id", userID, "job_id", jobID, "error", err)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true, err.Error())
		} else {
			slog.Info("RecalcBestEfforts completed", "user_id", userID, "job_id", jobID)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false, "")
		}
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status": "started",
		"job_id": jobID,
	})
}
