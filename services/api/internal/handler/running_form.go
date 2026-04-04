package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetRunningFormBaseline returns 90-day median running form metrics.
//
// GET /api/v1/running-form/baseline
func (h *Handler) GetRunningFormBaseline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	baseline, err := h.DB.GetRunningFormBaseline(ctx, userID)
	if err != nil {
		slog.Error("running form baseline: query failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "Daten konnten nicht geladen werden", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, baseline)
}

// GetRunningFormTrend returns per-activity running form values as a time series.
//
// GET /api/v1/running-form/trend
// Optional query params: from=2025-01-01&to=2026-04-04
func (h *Handler) GetRunningFormTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()

	var from, to *time.Time
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			to = &t
		}
	}

	points, err := h.DB.GetRunningFormTrend(ctx, userID, from, to)
	if err != nil {
		slog.Error("running form trend: query failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "Daten konnten nicht geladen werden", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"points": points,
	})
}
