package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// GetChangepoints reads changepoint data from analytics_cache.
// Query params: metric (default: hrv_rmssd), days (default: 180).
// Returns empty list if ML has not yet computed the data.
func (h *Handler) GetChangepoints(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "hrv_rmssd"
	}

	cacheKey := "changepoints." + metric
	data, err := h.DB.GetAnalyticsCache(r.Context(), userID, cacheKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{
				"metric":      metric,
				"changepoints": []any{},
				"computed":    false,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}

	var changepoints []json.RawMessage
	if err := json.Unmarshal(data, &changepoints); err != nil {
		changepoints = []json.RawMessage{}
	}
	if changepoints == nil {
		changepoints = []json.RawMessage{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"metric":       metric,
		"changepoints": changepoints,
		"computed":     true,
	})
}

// GetHRVExtended reads curated HRV metrics from user_daily_features for a given activity.
// Query param: activity_id (required).
func (h *Handler) GetHRVExtended(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	activityID := r.URL.Query().Get("activity_id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Missing Parameter",
			"activity_id query parameter is required", r.URL.Path)
		return
	}

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT feature_key, value_num, value_json
		 FROM user_daily_features
		 WHERE user_id = $1 AND activity_id = $2
		   AND feature_key LIKE 'hrv.%'
		 ORDER BY feature_key`,
		userID, activityID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Query Failed", err.Error(), r.URL.Path)
		return
	}
	defer rows.Close()

	features := map[string]any{}
	var fullBlob json.RawMessage
	for rows.Next() {
		var key string
		var numVal *float64
		var jsonVal []byte
		if err := rows.Scan(&key, &numVal, &jsonVal); err != nil {
			continue
		}
		if key == "hrv.full_blob" {
			fullBlob = json.RawMessage(jsonVal)
		} else if numVal != nil {
			// Strip the "hrv." prefix for cleaner response
			shortKey := key[4:]
			features[shortKey] = *numVal
		}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Query Failed", err.Error(), r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"activity_id": activityID,
		"features":    features,
		"full_blob":   fullBlob,
	})
}

// GetActivityFeatures reads TSFEL features from user_daily_features for a given activity.
// Query param: activity_id (required).
func (h *Handler) GetActivityFeatures(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	activityID := r.URL.Query().Get("activity_id")
	if activityID == "" {
		writeError(w, http.StatusBadRequest, "Missing Parameter",
			"activity_id query parameter is required", r.URL.Path)
		return
	}

	// Verify ownership
	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.UserID != userID {
		writeError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	// Return the compact full_blob
	row := h.DB.Pool.QueryRow(r.Context(),
		`SELECT value_json FROM user_daily_features
		 WHERE user_id = $1 AND activity_id = $2 AND feature_key = 'tsfel.full_blob'`,
		userID, activityID,
	)

	var blob []byte
	if err := row.Scan(&blob); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{
				"activity_id": activityID,
				"features":    map[string]any{},
				"computed":    false,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "Query Failed", err.Error(), r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"activity_id": activityID,
		"features":    json.RawMessage(blob),
		"computed":    true,
	})
}

// GetReadinessV2 reads readiness_v2 from analytics_cache.
// Returns 204 No Content when not yet computed.
func (h *Handler) GetReadinessV2(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "readiness_v2")
}

// GetAnomaliesV2 reads anomalies_v2 from analytics_cache.
// Returns 204 No Content when not yet computed.
func (h *Handler) GetAnomaliesV2(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "anomalies_v2")
}

// GetCorrelationsV2 reads correlations_v2 from analytics_cache.
// Returns 204 No Content when not yet computed.
func (h *Handler) GetCorrelationsV2(w http.ResponseWriter, r *http.Request) {
	h.serveInsightsCache(w, r, "correlations_v2")
}

// GetMLStatus proxies the ML service health endpoint and reports availability and latency.
func (h *Handler) GetMLStatus(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Available bool    `json:"available"`
		LatencyMs int64   `json:"latency_ms"`
		Error     *string `json:"error,omitempty"`
	}

	if h.ML == nil {
		msg := "ML client not configured"
		writeJSON(w, http.StatusOK, response{Available: false, LatencyMs: 0, Error: &msg})
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ok, err := h.ML.Health(ctx)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		msg := err.Error()
		writeJSON(w, http.StatusOK, response{Available: false, LatencyMs: latency, Error: &msg})
		return
	}

	writeJSON(w, http.StatusOK, response{Available: ok, LatencyMs: latency})
}
