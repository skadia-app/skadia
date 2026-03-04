package analytics

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// GetInsightsReadiness returns the ML-computed readiness score from the analytics cache.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsReadiness(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "readiness")
}

// mlAnomalyEntry is the shape the ML service stores in the "anomalies" cache key.
type mlAnomalyEntry struct {
	Date         string  `json:"date"`
	RmssdMs      float64 `json:"rmssd_ms"`
	Zscore       float64 `json:"zscore"`
	BaselineMean float64 `json:"baseline_mean"`
	BaselineStd  float64 `json:"baseline_std"`
}

// apiWarning is the shape the frontend Warning type expects.
type apiWarning struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	Message     string  `json:"message"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Explanation *string `json:"explanation"`
}

// GetInsightsWarnings reads the "anomalies" cache key written by the ML service
// and transforms the HRV anomaly list into the Warning[] shape the frontend expects.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsWarnings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	// ML service writes anomalies under the "anomalies" key.
	raw, err := h.DB.GetAnalyticsCache(r.Context(), userID, "anomalies")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}

	// Decode the list of HRV anomaly entries from the ML service.
	var anomalies []mlAnomalyEntry
	if err := json.Unmarshal(raw, &anomalies); err != nil {
		// Unexpected format — return empty list rather than an error.
		writeJSON(w, http.StatusOK, []apiWarning{})
		return
	}

	// Transform each HRV anomaly into a Warning for the frontend.
	warnings := make([]apiWarning, 0, len(anomalies))
	for _, a := range anomalies {
		zscore := a.Zscore
		var severity string
		switch {
		case zscore < -3.0:
			severity = "danger"
		case zscore < -2.5:
			severity = "warning"
		default:
			severity = "info"
		}
		explanation := fmt.Sprintf(
			"HRV am %s: %.0f ms (Baseline %.0f ± %.0f ms, Z-Score %.2f)",
			a.Date, a.RmssdMs, a.BaselineMean, a.BaselineStd, a.Zscore,
		)
		warnings = append(warnings, apiWarning{
			Type:        "hrv_anomaly",
			Severity:    severity,
			Message:     fmt.Sprintf("Ungewöhnlich niedrige HRV am %s (%.0f ms)", a.Date, a.RmssdMs),
			Value:       a.RmssdMs,
			Threshold:   a.BaselineMean + -2.0*a.BaselineStd,
			Explanation: &explanation,
		})
	}

	writeJSON(w, http.StatusOK, warnings)
}

// mlProjectionDict is the shape the ML service stores in the "projection" cache key.
type mlProjectionDict struct {
	Slope           *float64 `json:"slope"`
	CurrentCTL      *float64 `json:"current_ctl"`
	ProjectedCTL28d *float64 `json:"projected_ctl_28d"`
	Trend           *string  `json:"trend"`
	DataPoints      int      `json:"data_points"`
}

// apiProjection is the shape the frontend Projection type expects.
type apiProjection struct {
	Metric        string   `json:"metric"`
	Target        float64  `json:"target"`
	Current       float64  `json:"current"`
	PredictedDate *string  `json:"predicted_date"`
	OnTrack       bool     `json:"on_track"`
	WeeklyNeeded  float64  `json:"weekly_needed"`
	Explanation   *string  `json:"explanation"`
}

// GetInsightsProjection reads the "projection" cache key written by the ML service
// and transforms the CTL projection dict into the Projection[] array the frontend expects.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsProjection(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	raw, err := h.DB.GetAnalyticsCache(r.Context(), userID, "projection")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}

	// Decode the projection dict from the ML service.
	var proj mlProjectionDict
	if err := json.Unmarshal(raw, &proj); err != nil {
		writeJSON(w, http.StatusOK, []apiProjection{})
		return
	}

	// If there is no usable data, return an empty list so the frontend shows the empty-state.
	if proj.CurrentCTL == nil || proj.ProjectedCTL28d == nil || proj.DataPoints == 0 {
		writeJSON(w, http.StatusOK, []apiProjection{})
		return
	}

	currentCTL := *proj.CurrentCTL
	projectedCTL := *proj.ProjectedCTL28d
	onTrack := projectedCTL >= currentCTL

	// Predicted date: 28 days from today.
	predictedDate := time.Now().AddDate(0, 0, 28).Format("2006-01-02")

	// Weekly needed CTL gain to reach the projected value.
	weeklyNeeded := (projectedCTL - currentCTL) / 4.0 // 28d ≈ 4 weeks

	// Build a human-readable explanation from trend + slope.
	var explanationStr string
	if proj.Trend != nil && proj.Slope != nil {
		trendLabel := map[string]string{
			"aufbau":  "aufbauend",
			"abbau":   "abbauend",
			"plateau": "stabil (Plateau)",
		}[*proj.Trend]
		if trendLabel == "" {
			trendLabel = *proj.Trend
		}
		explanationStr = fmt.Sprintf(
			"Trend: %s (%.3f CTL/Tag) auf Basis von %d Datenpunkten",
			trendLabel, *proj.Slope, proj.DataPoints,
		)
	}
	var explanationPtr *string
	if explanationStr != "" {
		explanationPtr = &explanationStr
	}

	projections := []apiProjection{
		{
			Metric:        "ctl",
			Target:        projectedCTL,
			Current:       currentCTL,
			PredictedDate: &predictedDate,
			OnTrack:       onTrack,
			WeeklyNeeded:  weeklyNeeded,
			Explanation:   explanationPtr,
		},
	}

	writeJSON(w, http.StatusOK, projections)
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
