package handler

import (
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetHealthMonitoring returns health monitoring samples with auto-bucketing.
// GET /api/v1/health/monitoring?from=RFC3339&to=RFC3339&bucket=1m|15m|1h|1d
func (h *Handler) GetHealthMonitoring(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "from and to are required", r.URL.Path)
		return
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid from: "+err.Error(), r.URL.Path)
		return
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid to: "+err.Error(), r.URL.Path)
		return
	}

	// Auto-bucketing based on range
	duration := to.Sub(from)
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		switch {
		case duration <= 24*time.Hour:
			bucket = "1m"
		case duration <= 7*24*time.Hour:
			bucket = "15m"
		case duration <= 90*24*time.Hour:
			bucket = "1h"
		default:
			bucket = "1d"
		}
	}

	var results any
	switch bucket {
	case "1m":
		results, err = h.DB.GetMonitoringSamples(r.Context(), userID, from, to)
	case "15m", "1h":
		results, err = h.DB.GetMonitoringSamplesBucketed(r.Context(), userID, from, to, bucket)
	case "1d":
		results, err = h.DB.GetMonitoringDaily(r.Context(), userID, from, to)
	default:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid bucket: "+bucket, r.URL.Path)
		return
	}
	if err != nil {
		httputil.HandleDBErr(w, r, err, "get monitoring samples")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, results)
}

// GetHealthMonitoringDaily returns daily aggregated monitoring data from Continuous Aggregate.
// GET /api/v1/health/monitoring/daily?from=2006-01-02&to=2006-01-02
func (h *Handler) GetHealthMonitoringDaily(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" || toStr == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "from and to required", r.URL.Path)
		return
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid from: "+err.Error(), r.URL.Path)
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid to: "+err.Error(), r.URL.Path)
		return
	}
	to = to.Add(24*time.Hour - time.Nanosecond)
	results, err := h.DB.GetMonitoringDaily(r.Context(), userID, from, to)
	if err != nil {
		httputil.HandleDBErr(w, r, err, "get monitoring daily")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, results)
}
