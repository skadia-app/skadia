package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	goanalytics "github.com/skadia-app/skadia/services/api/internal/analytics"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// refreshSemaphores holds per-user mutexes to allow at most one concurrent
// RefreshInsights computation per user.
var (
	refreshMu   sync.Mutex
	refreshLock = map[string]struct{}{}
)

// GetInsightsReadiness returns the ML-computed readiness score from the analytics cache.
// Returns 204 No Content when the ML service has not yet computed the data.
func (h *Handler) GetInsightsReadiness(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "readiness")
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

// metricLabel maps internal metric names to human-readable German labels.
var metricLabel = map[string]string{
	"hrv_rmssd":  "HRV (RMSSD)",
	"resting_hr": "Ruhepuls",
	"sleep_score": "Schlafqualität",
	"spo2":       "SpO₂",
}

// GetInsightsWarnings reads the "anomalies" cache key written by the Go analytics
// package and transforms each AnomalyEntry into Warning[] entries for the frontend.
// Returns 204 No Content when no data has been computed yet.
func (h *Handler) GetInsightsWarnings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	raw, err := h.DB.GetAnalyticsCache(r.Context(), userID, "anomalies")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}

	// Decode []AnomalyEntry written by analytics.DetectAnomalies.
	type anomalyDetail struct {
		Metric       string  `json:"metric"`
		Value        float64 `json:"value"`
		ZScore       float64 `json:"zscore"`
		BaselineMean float64 `json:"baseline_mean"`
		BaselineStd  float64 `json:"baseline_std"`
	}
	type anomalyEntry struct {
		Date            string          `json:"date"`
		Severity        string          `json:"severity"`
		AffectedMetrics []string        `json:"affected_metrics"`
		Details         []anomalyDetail `json:"details"`
	}
	var anomalies []anomalyEntry
	if err := json.Unmarshal(raw, &anomalies); err != nil {
		httputil.WriteJSON(w, http.StatusOK, []apiWarning{})
		return
	}

	// Map Go analytics severity (low/medium/high) to frontend severity labels.
	sevMap := map[string]string{
		"low":    "info",
		"medium": "warning",
		"high":   "danger",
	}

	// One apiWarning per (day × metric) detail entry.
	warnings := make([]apiWarning, 0)
	for _, a := range anomalies {
		frontendSev := sevMap[a.Severity]
		if frontendSev == "" {
			frontendSev = "info"
		}
		for _, d := range a.Details {
			label := metricLabel[d.Metric]
			if label == "" {
				label = d.Metric
			}
			explanation := fmt.Sprintf(
				"%s am %s: %.2f (Baseline %.2f ± %.2f, Z-Score %.2f)",
				label, a.Date, d.Value, d.BaselineMean, d.BaselineStd, d.ZScore,
			)
			threshold := d.BaselineMean - 2.0*d.BaselineStd
			if d.ZScore > 0 {
				threshold = d.BaselineMean + 2.0*d.BaselineStd
			}
			warnings = append(warnings, apiWarning{
				Type:        d.Metric + "_anomaly",
				Severity:    frontendSev,
				Message:     fmt.Sprintf("Anomalie: %s am %s", label, a.Date),
				Value:       d.Value,
				Threshold:   threshold,
				Explanation: &explanation,
			})
		}
	}

	httputil.WriteJSON(w, http.StatusOK, warnings)
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
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	raw, err := h.DB.GetAnalyticsCache(r.Context(), userID, "projection")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}

	// Decode the projection dict from the ML service.
	var proj mlProjectionDict
	if err := json.Unmarshal(raw, &proj); err != nil {
		httputil.WriteJSON(w, http.StatusOK, []apiProjection{})
		return
	}

	// If there is no usable data, return an empty list so the frontend shows the empty-state.
	if proj.CurrentCTL == nil || proj.ProjectedCTL28d == nil || proj.DataPoints == 0 {
		httputil.WriteJSON(w, http.StatusOK, []apiProjection{})
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

	httputil.WriteJSON(w, http.StatusOK, projections)
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
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	data, err := h.DB.GetAnalyticsCache(r.Context(), userID, cacheKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// ML service has not yet computed this value — tell the client to retry later.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data) //nolint:errcheck
}

// RefreshInsights triggers re-computation of all analytics insights for the current user.
// The four Go analytics functions (readiness, anomalies, projection, correlations) are
// called directly in a background goroutine. Returns 202 Accepted immediately.
// At most one computation per user runs concurrently; additional requests are silently
// dropped (the in-flight computation will write fresh results).
func (h *Handler) RefreshInsights(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	// Acquire per-user lock — non-blocking. If a computation is already running, skip.
	refreshMu.Lock()
	_, alreadyRunning := refreshLock[userID]
	if !alreadyRunning {
		refreshLock[userID] = struct{}{}
	}
	refreshMu.Unlock()

	if !alreadyRunning {
		db := h.DB
		go func() {
			defer func() {
				refreshMu.Lock()
				delete(refreshLock, userID)
				refreshMu.Unlock()
			}()

			ctx := context.Background()

			if err := goanalytics.ComputeReadiness(ctx, db, userID); err != nil {
				slog.Error("RefreshInsights: ComputeReadiness failed", "user_id", userID, "err", err)
			}
			if err := goanalytics.DetectAnomalies(ctx, db, userID); err != nil {
				slog.Error("RefreshInsights: DetectAnomalies failed", "user_id", userID, "err", err)
			}
			if err := goanalytics.ComputeProjection(ctx, db, userID); err != nil {
				slog.Error("RefreshInsights: ComputeProjection failed", "user_id", userID, "err", err)
			}
			if err := goanalytics.ComputeCorrelations(ctx, db, userID); err != nil {
				slog.Error("RefreshInsights: ComputeCorrelations failed", "user_id", userID, "err", err)
			}
		}()
	}

	w.WriteHeader(http.StatusAccepted)
}
