package analytics

import (
	"bytes"
	"context"
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

// readinessMissingFactor matches the Python worker's missing_factors element
// shape: {key, reason, last_seen_at}. Spec §5.3.
type readinessMissingFactor struct {
	Key        string  `json:"key"`
	Reason     string  `json:"reason"`
	LastSeenAt *string `json:"last_seen_at"`
}

// readinessRefusal is the schema-complete refusal envelope. Every field is
// non-omit-empty so a TS interface generated 1:1 from this struct parses
// without excess-property errors. See Spec §5.3 / Plan Phase 5.
//
// Type notes (matching the Python worker's output):
//   - Score: *int (0–100 integer, never float)
//   - Confidence: *string ("high"|"medium"|"low")
//   - SignalModifier: *string (e.g. "rhr_changepoint_recent")
//   - PreferencesVersion: *string (ISO timestamp from user_readiness_preferences.updated_at)
//   - MissingFactors: []readinessMissingFactor ({key, reason, last_seen_at})
//
// Values are written as nil/zero when not applicable; encoding/json renders
// pointer-nil as `null` and an empty slice as `[]`.
type readinessRefusal struct {
	Refused            bool                     `json:"refused"`
	RefusalReason      string                   `json:"refusal_reason"`
	Score              *int                     `json:"score"`
	Level              *string                  `json:"level"`
	Factors            []any                    `json:"factors"`
	MissingFactors     []readinessMissingFactor  `json:"missing_factors"`
	ComputedAt         string                   `json:"computed_at"`
	DataWindowEnd      *string                  `json:"data_window_end"`
	PreferencesVersion *string                  `json:"preferences_version"`
	Confidence         *string                  `json:"confidence"`
	RecommendationCode *string                  `json:"recommendation_code"`
	SignalModifier     *string                  `json:"signal_modifier"`
	Degraded           *string                  `json:"degraded"`
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
		MissingFactors: []readinessMissingFactor{},
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
		// Cache miss: trigger recompute so the worker populates the row, then
		// serve service_unavailable rather than conflating this with a genuine
		// "no data ever" state. The frontend already handles service_unavailable
		// with the "Berechnung temporär nicht verfügbar" copy.
		if h.Invalidator != nil {
			if err := h.Invalidator.PublishAnalyticsInvalidate(userID); err != nil {
				slog.Warn("readiness cache miss: invalidate publish failed", "user_id", userID, "error", err)
			}
		}
		writeRefused(w, "service_unavailable", nil)
		return
	}

	// Preferences-version check (Spec §10.2): compare preferences_version
	// embedded in the cache JSON against the current user_readiness_preferences.updated_at.
	// If they differ the cache was computed under a stale pref — treat as degraded
	// and trigger a fresh recompute, regardless of expires_at.
	if mismatch := h.checkPreferencesVersionMismatch(r.Context(), userID, row.Data); mismatch {
		slog.Info("readiness preferences_version mismatch — serving degraded, triggering recompute",
			"user_id", userID)
		if h.Invalidator != nil {
			if err := h.Invalidator.PublishAnalyticsInvalidate(userID); err != nil {
				slog.Warn("readiness pref-version recompute publish failed", "user_id", userID, "error", err)
			}
		}
		writeStaleDegraded(w, row.Data)
		return
	}

	now := time.Now().UTC()
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

// cachePreferencesVersionShape is the minimal subset of the cache JSON that
// we need to extract preferences_version without unmarshal-remarshaling the
// full payload (which would strip unknown fields and violate the additive-only
// contract on other code paths).
//
// PreferencesVersion is captured as json.RawMessage so we can distinguish
// "field absent / explicit null" from "field present with a non-string value"
// (e.g. a stale cache row written by an older worker that used a numeric
// version). Non-string values must be treated as a mismatch — see M9.
type cachePreferencesVersionShape struct {
	PreferencesVersion json.RawMessage `json:"preferences_version"`
}

// checkPreferencesVersionMismatch returns true when the preferences_version
// stored in the cache JSON no longer matches the current
// user_readiness_preferences.updated_at. This means the cache was computed
// under a superseded preference and must be treated as stale-degraded.
//
// Null-vs-null is explicitly treated as a match: a cache computed when no
// pref row existed and a current state with no pref row are consistent.
//
// Parse-error policy (M9): if the cached preferences_version is present but
// malformed (wrong JSON type, unparseable timestamp), treat as mismatch and
// recompute. The conservative path is "force fresh", not "serve stale of
// unknown vintage" — a cache row whose type-contract doesn't match the spec
// has no trustworthy meaning. Pure DB read failures still return false: a
// transient DB outage shouldn't snowball into recompute storms.
func (h *Handler) checkPreferencesVersionMismatch(ctx context.Context, userID string, cacheData []byte) bool {
	// Extract preferences_version from cache JSON.
	var shape cachePreferencesVersionShape
	if err := json.Unmarshal(cacheData, &shape); err != nil {
		slog.Warn("readiness: cannot parse cache JSON for pref-version check — treating as mismatch",
			"user_id", userID, "error", err)
		return true
	}

	// Classify the raw preferences_version field.
	//   absent / explicit null  → cachedISO = "", cachedNull = true
	//   string                  → cachedISO = unquoted string, cachedNull = false
	//   anything else (number,  → invalid type → mismatch
	//                  bool,
	//                  array,
	//                  object)
	var cachedISO string
	cachedNull := true
	if len(shape.PreferencesVersion) > 0 && !bytes.Equal(bytes.TrimSpace(shape.PreferencesVersion), []byte("null")) {
		// Field is present and not null. Must be a JSON string per Spec §5.3.
		if err := json.Unmarshal(shape.PreferencesVersion, &cachedISO); err != nil {
			slog.Warn("readiness: preferences_version has non-string type — treating as mismatch",
				"user_id", userID, "raw", string(shape.PreferencesVersion))
			return true
		}
		cachedNull = false
	}

	// Load current pref row.
	pref, err := h.DB.GetUserReadinessPreference(ctx, userID)
	if err != nil {
		slog.Warn("readiness: pref-version DB read failed — skipping check", "user_id", userID, "error", err)
		return false
	}

	// Both null → computed and current both have no pref row → consistent.
	if cachedNull && pref == nil {
		return false
	}
	// One null, one non-null → mismatch.
	if cachedNull != (pref == nil) {
		return true
	}
	// Both non-null: parse and compare as time instants.
	// The worker writes updated_at.astimezone(utc).isoformat().replace("+00:00","Z")
	// which matches RFC3339Nano. The DB column is TIMESTAMPTZ so both sides
	// represent the same physical instant — time.Parse + Equal handles any
	// representation differences (trailing zeros, 'Z' vs '+00:00').
	cachedTime, err := time.Parse(time.RFC3339Nano, cachedISO)
	if err != nil {
		// Try plain RFC3339 as fallback.
		cachedTime, err = time.Parse(time.RFC3339, cachedISO)
		if err != nil {
			slog.Warn("readiness: cannot parse cached preferences_version timestamp — treating as mismatch",
				"user_id", userID, "value", cachedISO, "error", err)
			return true
		}
	}
	return !cachedTime.Equal(pref.UpdatedAt)
}

// writeStaleDegraded splices `,"degraded":"service_unavailable"` into the
// existing cache-hit JSON object at the byte level, avoiding a full
// unmarshal-remarshal round trip that loses key order and may reformat numbers.
//
// Edge cases:
//   - Empty object `{}` → replaced with `{"degraded":"service_unavailable"}`.
//   - Non-object payload → falls back to a refused envelope.
func writeStaleDegraded(w http.ResponseWriter, data []byte) {
	const suffix = `,"degraded":"service_unavailable"}`
	body := bytes.TrimSpace(data)
	if len(body) < 2 || body[0] != '{' || body[len(body)-1] != '}' {
		slog.Warn("readiness stale body not a JSON object — falling back to refused envelope")
		writeRefused(w, "service_unavailable", nil)
		return
	}
	var spliced []byte
	if body[len(body)-2] == '{' {
		// Empty object: {} → {"degraded":"service_unavailable"}
		spliced = []byte(`{"degraded":"service_unavailable"}`)
	} else {
		// Normal case: append before closing brace.
		spliced = append(body[:len(body)-1:len(body)-1], []byte(suffix)...)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(spliced)
}

