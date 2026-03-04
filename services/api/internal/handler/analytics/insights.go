package analytics

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// GetInsightsReadiness returns the ML-computed readiness score from the analytics cache.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsReadiness(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "readiness")
}

// GetInsightsWarnings returns ML-generated training warnings from the analytics cache.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsWarnings(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "warnings")
}

// GetInsightsProjection returns ML-computed performance projections from the analytics cache.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsProjection(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "projection")
}

// GetInsightsClusters returns ML-computed activity cluster analysis from the analytics cache.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsClusters(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "clusters")
}

// GetInsightsCorrelations returns ML-computed metric correlations from the analytics cache.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsCorrelations(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "correlations")
}

// serveInsightsCache is a shared helper: reads the given cache key and writes
// the raw JSONB payload to the response, or 204 when no cache entry exists yet.
func (h *Handler) serveInsightsCache(w http.ResponseWriter, r *http.Request, cacheKey string) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	data, err := h.DB.GetAnalyticsCache(r.Context(), userID, cacheKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ML service has not yet computed this value — tell the client to retry later.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data) //nolint:errcheck
}

// RefreshInsights triggers re-computation of all analytics insights for the current user
// by publishing a skadia.analytics.invalidate NATS event to the ML service.
// Returns 202 Accepted immediately; the actual recomputation happens asynchronously.
func (h *Handler) RefreshInsights(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	if h.Publisher != nil {
		h.Publisher.PublishAnalyticsInvalidate(userID)
	}

	w.WriteHeader(http.StatusAccepted)
}
