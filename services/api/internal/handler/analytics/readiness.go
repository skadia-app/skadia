package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// STALE_HARD_CAP is the maximum age at which a stale cache entry is still
// served (with a stale-while-revalidate trigger). Beyond this, the response
// becomes a hard refusal with reason="service_unavailable".
const STALE_HARD_CAP = 48 * time.Hour

// readinessTriggerWindow is the minimum interval between two
// PublishAnalyticsInvalidate calls for the same user_id from the readiness
// stale/mismatch path. Within this window, repeated GETs return the existing
// degraded body without re-publishing, breaking the trigger-loop a
// deterministic worker failure would otherwise cause: every GET would publish,
// the worker would re-fail, the fallback envelope would carry
// preferences_version=null which mismatches every real pref row, the next GET
// would publish again, and so on.
const readinessTriggerWindow = 60 * time.Second

// readinessTriggerCap caps the in-memory user→last-trigger map. Above this
// size, half of the entries are evicted (oldest first). 10k slots × ~48 bytes
// (UUID string + time.Time) ≈ 480 KB worst-case — bounded.
const readinessTriggerCap = 10_000

// readinessTriggerThrottle rate-limits PublishAnalyticsInvalidate calls per
// user_id. Concurrency-safe; lazy-initialized via sync.Once on first use so
// Handlers built via struct literal (test code) work without ceremony.
type readinessTriggerThrottle struct {
	mu        sync.Mutex
	lastFired map[string]time.Time
}

// shouldTrigger returns true exactly once per user_id per readinessTriggerWindow.
// Uses now as the clock to keep tests deterministic.
func (t *readinessTriggerThrottle) shouldTrigger(userID string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastFired == nil {
		t.lastFired = make(map[string]time.Time)
	}
	if last, ok := t.lastFired[userID]; ok && now.Sub(last) < readinessTriggerWindow {
		return false
	}
	t.evictIfFullLocked(now)
	t.lastFired[userID] = now
	return true
}

// evictIfFullLocked drops entries older than readinessTriggerWindow when the
// map exceeds readinessTriggerCap. Caller must hold t.mu. Worst case: a burst
// of 10k unique users within window — we then drop only the truly-stale ones,
// keeping the throttle effective for the active set.
func (t *readinessTriggerThrottle) evictIfFullLocked(now time.Time) {
	if len(t.lastFired) < readinessTriggerCap {
		return
	}
	cutoff := now.Add(-readinessTriggerWindow)
	for uid, ts := range t.lastFired {
		if ts.Before(cutoff) {
			delete(t.lastFired, uid)
		}
	}
	// If everything is still in-window, force-drop arbitrary half to bound
	// memory. Throttling becomes best-effort under sustained 10k+ unique
	// active users — acceptable for selfhost scale targets.
	if len(t.lastFired) >= readinessTriggerCap {
		drop := len(t.lastFired) / 2
		for uid := range t.lastFired {
			if drop == 0 {
				break
			}
			delete(t.lastFired, uid)
			drop--
		}
	}
}

// readinessThrottle returns the Handler's throttle, initializing on first use.
func (h *Handler) readinessThrottle() *readinessTriggerThrottle {
	h.readinessThrottleOnce.Do(func() {
		h.readinessThrottleInst = &readinessTriggerThrottle{}
	})
	return h.readinessThrottleInst
}

// publishReadinessInvalidate wraps Invalidator.PublishAnalyticsInvalidate
// behind the per-user throttle. Returns (triggered, error). triggered=false
// means the call was suppressed by the throttle, not a failure.
func (h *Handler) publishReadinessInvalidate(userID string) (bool, error) {
	if h.Invalidator == nil {
		return false, nil
	}
	if !h.readinessThrottle().shouldTrigger(userID, time.Now()) {
		return false, nil
	}
	return true, h.Invalidator.PublishAnalyticsInvalidate(userID)
}

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
		if _, err := h.publishReadinessInvalidate(userID); err != nil {
			slog.Warn("readiness cache miss: invalidate publish failed", "user_id", userID, "error", err)
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
		if _, err := h.publishReadinessInvalidate(userID); err != nil {
			slog.Warn("readiness pref-version recompute publish failed", "user_id", userID, "error", err)
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
	triggered, err := h.publishReadinessInvalidate(userID)
	if err != nil {
		slog.Warn("readiness invalidate publish failed", "user_id", userID, "error", err)
		writeStaleDegraded(w, row.Data)
		return
	}
	if !triggered {
		// Throttled — a recompute for this user fired within the last
		// readinessTriggerWindow. Serve the stale body decorated as degraded
		// so the client sees a clear "computation pending" signal instead of
		// a fresh-looking row that would silently be repeated indefinitely.
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
//   - Body already contains a top-level `degraded` key with a non-null value:
//     pass body through unchanged (M3). The worker's authoritative degraded
//     value (e.g. "compute_pending") wins over the handler's stale-decoration.
//     Splicing a second `degraded` would produce a duplicate-key object
//     whose final value is parser-implementation-defined.
//   - Body has `"degraded": null` (the spec-required default written by the
//     worker on every fresh compute): rewrite the null in place to
//     "service_unavailable" so the response has exactly one degraded key.
func writeStaleDegraded(w http.ResponseWriter, data []byte) {
	const suffix = `,"degraded":"service_unavailable"}`
	body := bytes.TrimSpace(data)
	if len(body) < 2 || body[0] != '{' || body[len(body)-1] != '}' {
		slog.Warn("readiness stale body not a JSON object — falling back to refused envelope")
		writeRefused(w, "service_unavailable", nil)
		return
	}

	// Top-level key probe (M3). Using map[string]json.RawMessage ensures we
	// match a real JSON key and ignore any "degraded" substring that may
	// occur inside a string value.
	if topKeys, err := parseTopKeys(body); err == nil {
		if raw, present := topKeys["degraded"]; present {
			if isJSONNull(raw) {
				// degraded:null → rewrite that null to "service_unavailable"
				// in place; preserves key order, no duplicate.
				if rewritten, ok := rewriteDegradedNull(data); ok {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(rewritten)
					return
				}
				// Fall through to splice if rewrite couldn't locate the null
				// (string value containing "degraded":null is the pathological
				// case; the parser-validated rewrite above is robust to it).
			} else {
				// Worker set a non-null degraded value — that's authoritative.
				// Pass body verbatim.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
		}
	}
	// Note: an unmarshal failure here doesn't change the splice path — the
	// outer-shape check above already confirmed `{...}` framing.

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

// parseTopKeys extracts only the top-level keys of a JSON object. Used to
// detect well-formed JSON without paying the full deep-parse cost.
func parseTopKeys(body []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// isJSONNull reports whether raw represents the literal JSON `null` (after
// trimming whitespace). RawMessage preserves whitespace from the source.
func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// rewriteDegradedNull replaces the value of a top-level `"degraded": null`
// with `"service_unavailable"`. Walks the JSON byte-by-byte at depth 1 so a
// nested "degraded" key inside a factor object never matches.
//
// Returns the rewritten body and true on success; the original body and
// false if no top-level `"degraded": null` was found (caller falls back to
// splice).
func rewriteDegradedNull(data []byte) ([]byte, bool) {
	const target = `"degraded"`
	const replacement = `"service_unavailable"`

	// Walk bytes tracking object/array depth and string state.
	depth := 0
	i := 0
	n := len(data)
	for i < n {
		c := data[i]
		switch c {
		case '"':
			// Scan to end of string, honoring escapes.
			j := i + 1
			for j < n {
				if data[j] == '\\' {
					j += 2
					continue
				}
				if data[j] == '"' {
					break
				}
				j++
			}
			if j >= n {
				return data, false
			}
			// We're at depth==1 inside the top-level object on a key. Check
			// for the target key.
			if depth == 1 && j-i == len(target)-1 && bytes.Equal(data[i:j+1], []byte(target)) {
				// Skip to ':'
				k := j + 1
				for k < n && (data[k] == ' ' || data[k] == '\t' || data[k] == '\n' || data[k] == '\r') {
					k++
				}
				if k >= n || data[k] != ':' {
					return data, false
				}
				k++
				// Skip whitespace.
				for k < n && (data[k] == ' ' || data[k] == '\t' || data[k] == '\n' || data[k] == '\r') {
					k++
				}
				// Check for `null`.
				if k+4 <= n && string(data[k:k+4]) == "null" {
					out := make([]byte, 0, n+len(replacement))
					out = append(out, data[:k]...)
					out = append(out, []byte(replacement)...)
					out = append(out, data[k+4:]...)
					return out, true
				}
				// Not null — caller's parseTopKeys decided the value is non-null
				// and the caller path handles that. Stop searching.
				return data, false
			}
			i = j + 1
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		i++
	}
	return data, false
}

