package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// ListActiveJobs gibt laufende Background-Jobs + Anzahl pending Enrichments zurück.
//
// GET /api/v1/jobs/active
func (h *Handler) ListActiveJobs(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	jobs, err := h.DB.ListActiveBackgroundJobs(r.Context(), userID)
	if err != nil {
		slog.Error("list active jobs failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list jobs", r.URL.Path)
		return
	}

	// Include active Garmin + Strava import jobs
	if garminJobs, err := h.DB.ListActiveGarminImportJobs(r.Context(), userID); err == nil {
		jobs = append(jobs, garminJobs...)
	} else {
		slog.Warn("list active garmin import jobs failed", "error", err)
	}
	if stravaJobs, err := h.DB.ListActiveStravaImportJobs(r.Context(), userID); err == nil {
		jobs = append(jobs, stravaJobs...)
	} else {
		slog.Warn("list active strava import jobs failed", "error", err)
	}

	pending, err := h.DB.CountPendingEnrichments(r.Context(), userID)
	if err != nil {
		slog.Warn("count pending enrichments failed", "error", err)
		pending = 0
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"jobs":                jobs,
		"pending_enrichments": pending,
	})
}
