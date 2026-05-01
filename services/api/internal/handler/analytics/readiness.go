package analytics

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// STALE_HARD_CAP is the maximum age at which a stale cache entry is still
// served (with a stale-while-revalidate trigger). Beyond this, the response
// becomes a hard refusal with reason="service_unavailable".
const STALE_HARD_CAP = 48 * time.Hour

// readinessRefusal is the schema-complete refusal envelope. Every field is
// non-omit-empty so a TS interface generated 1:1 from this struct parses
// without excess-property errors. See Spec §5.3 / Plan Phase 5.
//
// Values are written as nil/zero when not applicable; encoding/json renders
// pointer-nil as `null` and an empty []string as `[]`.
type readinessRefusal struct {
	Refused            bool      `json:"refused"`
	RefusalReason      string    `json:"refusal_reason"`
	Score              *float64  `json:"score"`
	Level              *string   `json:"level"`
	Factors            []any     `json:"factors"`
	MissingFactors     []string  `json:"missing_factors"`
	ComputedAt         string    `json:"computed_at"`
	DataWindowEnd      *string   `json:"data_window_end"`
	PreferencesVersion *int      `json:"preferences_version"`
	Confidence         *float64  `json:"confidence"`
	RecommendationCode *string   `json:"recommendation_code"`
	SignalModifier     *float64  `json:"signal_modifier"`
	Degraded           *string   `json:"degraded"`
}

// writeRefused writes a 200 response carrying the schema-complete refusal
// envelope. lastComputedAt is taken when present (e.g. for hard-cap stale
// rows); otherwise NOW() in UTC RFC3339.
func writeRefused(w http.ResponseWriter, reason string, lastComputedAt *time.Time) {
	var ts time.Time
	if lastComputedAt != nil {
		ts = *lastComputedAt
	} else {
		ts = time.Now().UTC()
	}
	body := readinessRefusal{
		Refused:        true,
		RefusalReason:  reason,
		Factors:        []any{},
		MissingFactors: []string{},
		ComputedAt:     ts.UTC().Format(time.RFC3339),
	}
	httputil.WriteJSON(w, http.StatusOK, body)
}

// GetReadiness serves the consolidated readiness payload from
// analytics_cache (key="readiness"), implementing the stale-while-revalidate
// behavior defined in the consolidation spec.
func (h *Handler) GetReadiness(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	row, err := h.DB.GetAnalyticsCacheWithMeta(r.Context(), userID, "readiness")
	if err != nil {
		slog.Error("readiness cache read failed", "user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "cache read failed", r.URL.Path)
		return
	}
	if row == nil {
		writeRefused(w, "no_data", nil)
		return
	}

	now := time.Now()
	stale := row.ExpiresAt != nil && row.ExpiresAt.Before(now)
	if !stale {
		// Fresh cache hit — write payload verbatim.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(row.Data)
		return
	}

	// Stale path: enforce hard cap first.
	if now.Sub(row.ComputedAt) > STALE_HARD_CAP {
		ts := row.ComputedAt
		writeRefused(w, "service_unavailable", &ts)
		return
	}

	// Stale-while-revalidate: trigger recompute, otherwise fall back to
	// degraded passthrough.
	if h.Invalidator == nil {
		slog.Warn("readiness stale but no invalidator configured", "user_id", userID)
		writeStaleDegraded(w, row.Data)
		return
	}
	if err := h.Invalidator.PublishAnalyticsInvalidate(userID); err != nil {
		slog.Warn("readiness invalidate publish failed", "user_id", userID, "error", err)
		writeStaleDegraded(w, row.Data)
		return
	}

	// Recompute triggered: ship the stale payload unchanged.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(row.Data)
}

// writeStaleDegraded splices "degraded":"service_unavailable" into the
// existing JSON object without losing its other fields.
func writeStaleDegraded(w http.ResponseWriter, data json.RawMessage) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		// Fall back to a refused envelope when the payload is not a JSON object.
		slog.Warn("readiness stale body not a JSON object", "error", err)
		writeRefused(w, "service_unavailable", nil)
		return
	}
	m["degraded"] = "service_unavailable"
	httputil.WriteJSON(w, http.StatusOK, m)
}

