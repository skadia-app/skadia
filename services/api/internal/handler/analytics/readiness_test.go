//go:build integration

package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/event"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// readinessTestPool opens a pool against TEST_DATABASE_URL or skips.
func readinessTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping readiness handler integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedReadinessTestUser inserts a user, registers cleanup for users +
// analytics_cache + user_readiness_preferences.
func seedReadinessTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])
	var uid string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"readiness-handler-"+suffix,
		"readiness-handler-"+suffix+"@test.localhost",
	).Scan(&uid)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM analytics_cache WHERE user_id = $1`, uid)
		_, _ = pool.Exec(ctx, `DELETE FROM user_readiness_preferences WHERE user_id = $1`, uid)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, uid)
	})
	return uid
}

// seedReadinessRow writes an analytics_cache row directly with explicit
// computed_at and expires_at timestamps. Used to set up stale/fresh fixtures.
func seedReadinessRow(t *testing.T, pool *pgxpool.Pool, uid string, body json.RawMessage, computedAt time.Time, expiresAt *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO analytics_cache (user_id, cache_key, data, computed_at, expires_at)
		 VALUES ($1, 'readiness', $2, $3, $4)
		 ON CONFLICT (user_id, cache_key) DO UPDATE
		 SET data = EXCLUDED.data, computed_at = EXCLUDED.computed_at, expires_at = EXCLUDED.expires_at`,
		uid, body, computedAt, expiresAt,
	)
	if err != nil {
		t.Fatalf("seed readiness row: %v", err)
	}
}

func newReadinessTestHandler(t *testing.T, inv event.AnalyticsInvalidator) (*Handler, *pgxpool.Pool) {
	pool := readinessTestPool(t)
	return &Handler{DB: &database.DB{Pool: pool}, Invalidator: inv}, pool
}

func reqWithUser(uid string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/analytics/readiness", nil)
	return r.WithContext(context.WithValue(r.Context(), middleware.UserIDKey, uid))
}

// TestReadiness_CacheHit — fresh entry served verbatim, no degraded field.
func TestReadiness_CacheHit(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	// Spec §5.3: score is int (0–100), level is enum critical|low|moderate|good|optimal,
	// preferences_version is ISO timestamp (or null when no pref row exists).
	body := json.RawMessage(`{"refused":false,"score":72,"level":"good","factors":[],"missing_factors":[],"computed_at":"2026-04-30T12:00:00Z","data_window_end":null,"preferences_version":null,"confidence":"high","recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(6 * time.Hour)
	seedReadinessRow(t, pool, uid, body, now.Add(-1*time.Hour), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["score"].(float64) != 72 {
		t.Errorf("score = %v, want 72", got["score"])
	}
	// Fresh hit must NOT splice degraded — the body's literal null is fine
	// (json.Unmarshal yields nil interface for explicit null), but we must NOT
	// have a non-null "service_unavailable" string here.
	if v, ok := got["degraded"]; ok && v != nil {
		t.Errorf("degraded = %v, want null/absent", v)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("invalidator was called %d times on fresh hit", len(fake.Calls))
	}
}

// TestReadiness_Miss — no cache row → refused with reason=service_unavailable and
// invalidate published so the worker recomputes. The "no_data" reason is reserved
// for genuine blank accounts; a cache miss after a fresh deploy must not conflate
// those two states.
func TestReadiness_Miss(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["refused"] != true {
		t.Errorf("refused = %v, want true", got["refused"])
	}
	if got["refusal_reason"] != "service_unavailable" {
		t.Errorf("refusal_reason = %v, want service_unavailable", got["refusal_reason"])
	}
	// Schema-completeness: required nullable fields must be present.
	for _, k := range []string{"score", "level", "factors", "missing_factors", "computed_at", "data_window_end", "preferences_version", "confidence", "recommendation_code", "signal_modifier", "degraded"} {
		if _, ok := got[k]; !ok {
			t.Errorf("refused envelope missing field %q", k)
		}
	}
	// Cache miss must trigger a recompute publish so the worker fills the row.
	if len(fake.Calls) != 1 || fake.Calls[0] != uid {
		t.Errorf("invalidator calls = %v, want [%s]", fake.Calls, uid)
	}
}

// TestReadiness_StaleInvalidatorOK — stale cache + healthy invalidator returns
// the stale body unchanged AND records the publish call.
func TestReadiness_StaleInvalidatorOK(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	body := json.RawMessage(`{"refused":false,"score":50,"level":"moderate","factors":[],"missing_factors":[],"computed_at":"2026-04-30T10:00:00Z","data_window_end":null,"preferences_version":null,"confidence":"medium","recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(-1 * time.Hour)
	seedReadinessRow(t, pool, uid, body, now.Add(-2*time.Hour), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["score"].(float64) != 50 {
		t.Errorf("score = %v, want 50", got["score"])
	}
	if v, ok := got["degraded"]; ok && v != nil && v != "" {
		// Original body had degraded:null; staleOK path passes through unchanged.
		if v != nil {
			t.Errorf("degraded = %v, want null on stale-but-revalidating", v)
		}
	}
	if len(fake.Calls) != 1 || fake.Calls[0] != uid {
		t.Errorf("invalidator calls = %v, want [%s]", fake.Calls, uid)
	}
}

// TestReadiness_StaleInvalidatorErr — invalidator publish fails → degraded
// splice into the existing body.
func TestReadiness_StaleInvalidatorErr(t *testing.T) {
	fake := &event.FakeInvalidator{NextErr: errors.New("publish failed")}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	body := json.RawMessage(`{"refused":false,"score":40,"level":"low","factors":[],"missing_factors":[],"computed_at":"2026-04-30T10:00:00Z","data_window_end":null,"preferences_version":null,"confidence":"low","recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(-30 * time.Minute)
	seedReadinessRow(t, pool, uid, body, now.Add(-2*time.Hour), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["score"].(float64) != 40 {
		t.Errorf("score = %v, want 40", got["score"])
	}
	if got["degraded"] != "service_unavailable" {
		t.Errorf("degraded = %v, want service_unavailable", got["degraded"])
	}
}

// TestReadiness_StaleNoInvalidator — Handler.Invalidator==nil → degraded
// splice (no publish attempted).
func TestReadiness_StaleNoInvalidator(t *testing.T) {
	h, pool := newReadinessTestHandler(t, nil)
	uid := seedReadinessTestUser(t, pool)

	body := json.RawMessage(`{"refused":false,"score":33,"level":"low","factors":[],"missing_factors":[],"computed_at":"2026-04-30T09:00:00Z","data_window_end":null,"preferences_version":null,"confidence":"low","recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(-15 * time.Minute)
	seedReadinessRow(t, pool, uid, body, now.Add(-3*time.Hour), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["degraded"] != "service_unavailable" {
		t.Errorf("degraded = %v, want service_unavailable", got["degraded"])
	}
	if got["score"].(float64) != 33 {
		t.Errorf("score = %v, want 33 (stale body preserved)", got["score"])
	}
}

// TestReadiness_StaleHardCap — older than STALE_HARD_CAP → refused envelope
// with reason=service_unavailable and computed_at from the row.
func TestReadiness_StaleHardCap(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	body := json.RawMessage(`{"refused":false,"score":80,"level":"optimal","factors":[],"missing_factors":[],"computed_at":"2026-04-28T10:00:00Z","data_window_end":null,"preferences_version":null,"confidence":"high","recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(-50 * time.Hour)
	computedAt := now.Add(-50 * time.Hour)
	seedReadinessRow(t, pool, uid, body, computedAt, &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["refused"] != true {
		t.Errorf("refused = %v, want true", got["refused"])
	}
	if got["refusal_reason"] != "service_unavailable" {
		t.Errorf("refusal_reason = %v, want service_unavailable", got["refusal_reason"])
	}
	// computed_at should reflect the cache row's computed_at (within 1 minute).
	gotTS, err := time.Parse(time.RFC3339, got["computed_at"].(string))
	if err != nil {
		t.Fatalf("parse computed_at: %v", err)
	}
	if delta := computedAt.Sub(gotTS); delta < -time.Minute || delta > time.Minute {
		t.Errorf("computed_at = %v, want close to %v", gotTS, computedAt)
	}
	if len(fake.Calls) != 0 {
		t.Errorf("hard-cap path must not publish; calls = %v", fake.Calls)
	}
}

// TestReadiness_Unauth — no user_id in context → 401.
func TestReadiness_Unauth(t *testing.T) {
	h, _ := newReadinessTestHandler(t, &event.FakeInvalidator{})

	r := httptest.NewRequest(http.MethodGet, "/analytics/readiness", nil)
	rr := httptest.NewRecorder()
	h.GetReadiness(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401; body: %s", rr.Code, rr.Body.String())
	}
}

// TestReadiness_AdditiveSchemaPassthrough — validates the additive-only cache
// schema contract (Spec §12.5): the handler MUST pass through unknown fields
// verbatim without stripping them. An old frontend build reading a cache row
// written by a newer worker must not lose any data it doesn't yet know about.
//
// Additive-only contract:
//   - Fields are NEVER renamed or removed from the cache JSONB.
//   - New fields are ONLY added; older readers skip them safely.
//   - The handler MUST NOT unmarshal+remarshal through a typed struct on the
//     fresh-cache path — doing so would strip unknown keys silently.
//
// If this test fails it means GetReadiness has introduced a normalization step
// on the fresh-cache path that destroys forward compatibility.
func TestReadiness_AdditiveSchemaPassthrough(t *testing.T) {
	h, pool := newReadinessTestHandler(t, &event.FakeInvalidator{})
	uid := seedReadinessTestUser(t, pool)

	// Seed a cache row containing a field that does not exist in the current
	// ReadinessResponse schema — simulating a row written by a future worker.
	body := json.RawMessage(`{` +
		`"refused":false,` +
		`"refusal_reason":null,` +
		`"score":73,` +
		`"level":"good",` +
		`"factors":[],` +
		`"missing_factors":[],` +
		`"computed_at":"2026-05-01T08:00:00Z",` +
		`"data_window_end":null,` +
		`"preferences_version":null,` +
		`"confidence":"high",` +
		`"recommendation_code":null,` +
		`"signal_modifier":null,` +
		`"degraded":null,` +
		`"future_field":42` +
		`}`)

	now := time.Now()
	expires := now.Add(6 * time.Hour)
	seedReadinessRow(t, pool, uid, body, now.Add(-30*time.Minute), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// Core fields must survive.
	if score, _ := got["score"].(float64); score != 73 {
		t.Errorf("score = %v, want 73", got["score"])
	}
	if level, _ := got["level"].(string); level != "good" {
		t.Errorf("level = %v, want good", got["level"])
	}

	// The unknown future field must be present and carry the original value.
	// Absence means the handler normalised the payload through a typed struct,
	// breaking forward compatibility for older frontend builds.
	futureVal, ok := got["future_field"]
	if !ok {
		t.Fatal("additive-schema violation: 'future_field' was stripped by GetReadiness — " +
			"fresh-cache path must write row.Data verbatim without unmarshal/remarshal")
	}
	if futureVal.(float64) != 42 {
		t.Errorf("future_field = %v, want 42", futureVal)
	}
}

// seedReadinessPref inserts a user_readiness_preferences row with an explicit
// updated_at value so we can control the pref-version comparison.
func seedReadinessPref(t *testing.T, pool *pgxpool.Pool, uid, source string, updatedAt time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO user_readiness_preferences (user_id, preferred_source, updated_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id) DO UPDATE
		 SET preferred_source = EXCLUDED.preferred_source,
		     updated_at       = EXCLUDED.updated_at`,
		uid, source, updatedAt,
	)
	if err != nil {
		t.Fatalf("seed pref: %v", err)
	}
}

// TestReadiness_PreferencesVersionMismatch — cache row carries an old
// preferences_version; current pref.updated_at is newer.
// GetReadiness must return degraded="service_unavailable" and publish invalidate.
func TestReadiness_PreferencesVersionMismatch(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	// Seed the pref row with a known updated_at.
	oldPrefTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newPrefTime := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedReadinessPref(t, pool, uid, "skadia", newPrefTime)

	// Seed the cache row with preferences_version pointing to the OLD time.
	oldPrefISO := oldPrefTime.UTC().Format(time.RFC3339Nano)
	body := json.RawMessage(`{"refused":false,"score":65,"level":"moderate","factors":[],"missing_factors":[],"computed_at":"2026-04-30T10:00:00Z","data_window_end":null,"preferences_version":"` + oldPrefISO + `","confidence":null,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(6 * time.Hour) // fresh by expires_at, but stale by pref-version
	seedReadinessRow(t, pool, uid, body, now.Add(-1*time.Hour), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["degraded"] != "service_unavailable" {
		t.Errorf("degraded = %v, want service_unavailable (pref-version mismatch)", got["degraded"])
	}
	// Original score must survive (stale body is passed through, not replaced).
	if got["score"].(float64) != 65 {
		t.Errorf("score = %v, want 65 (stale body preserved)", got["score"])
	}
	// Must publish invalidate so worker recomputes with the new pref.
	if len(fake.Calls) != 1 || fake.Calls[0] != uid {
		t.Errorf("invalidator calls = %v, want [%s]", fake.Calls, uid)
	}
}

// TestReadiness_TriggerLoopThrottle — two GETs for the same user within
// the readinessTriggerWindow must produce at most one
// PublishAnalyticsInvalidate. Without throttling, a deterministically-failing
// worker would produce a runaway loop: every GET → publish → fail →
// preferences_version=null in fallback envelope → mismatch → publish → ...
//
// Uses the stale-while-revalidate path (expires_at in the past) which is
// the most common trigger site.
func TestReadiness_TriggerLoopThrottle(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	body := json.RawMessage(`{"refused":false,"score":50,"level":"moderate","factors":[],"missing_factors":[],"computed_at":"2026-04-30T10:00:00Z","data_window_end":null,"preferences_version":null,"confidence":"medium","recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(-1 * time.Hour) // stale
	seedReadinessRow(t, pool, uid, body, now.Add(-2*time.Hour), &expires)

	// First GET — publishes.
	rr1 := httptest.NewRecorder()
	h.GetReadiness(rr1, reqWithUser(uid))
	if rr1.Code != http.StatusOK {
		t.Fatalf("GET#1 status %d, want 200", rr1.Code)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("after GET#1, invalidator calls = %d, want 1", len(fake.Calls))
	}

	// Second GET, immediately — must be throttled (no second publish).
	rr2 := httptest.NewRecorder()
	h.GetReadiness(rr2, reqWithUser(uid))
	if rr2.Code != http.StatusOK {
		t.Fatalf("GET#2 status %d, want 200", rr2.Code)
	}
	if len(fake.Calls) != 1 {
		t.Errorf("after GET#2, invalidator calls = %d, want 1 (second call must be throttled)", len(fake.Calls))
	}
	// Body of GET#2 must still be useful — the throttled path returns the
	// stale body with degraded spliced in.
	var got map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET#2: %v", err)
	}
	if got["degraded"] != "service_unavailable" {
		t.Errorf("GET#2 degraded = %v, want service_unavailable", got["degraded"])
	}
}

// TestReadinessTriggerThrottle_WindowExpiry — after the window has elapsed,
// a fresh trigger fires for the same user_id. Uses shouldTrigger directly
// with an explicit clock so the test stays fast.
func TestReadinessTriggerThrottle_WindowExpiry(t *testing.T) {
	throttle := &readinessTriggerThrottle{}
	uid := "user-1"
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if !throttle.shouldTrigger(uid, t0) {
		t.Fatal("first call must trigger")
	}
	if throttle.shouldTrigger(uid, t0.Add(30*time.Second)) {
		t.Error("call within window must be suppressed")
	}
	if !throttle.shouldTrigger(uid, t0.Add(61*time.Second)) {
		t.Error("call after window must trigger again")
	}
}

// TestReadinessTriggerThrottle_MemoryCap — when the map exceeds
// readinessTriggerCap, eviction keeps it bounded. Uses a tiny synthetic
// load (cap+1 unique users) and verifies the map shrinks.
func TestReadinessTriggerThrottle_MemoryCap(t *testing.T) {
	throttle := &readinessTriggerThrottle{}
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// Fill to cap with old entries.
	for i := 0; i < readinessTriggerCap; i++ {
		throttle.shouldTrigger(fmt.Sprintf("u-%d", i), t0)
	}
	if len(throttle.lastFired) != readinessTriggerCap {
		t.Fatalf("pre-eviction size = %d, want %d", len(throttle.lastFired), readinessTriggerCap)
	}

	// One more user, far enough in the future that all prior entries are
	// outside the window — eviction should drop them all and admit only
	// the new one (plus the in-window new caller).
	throttle.shouldTrigger("u-new", t0.Add(5*time.Minute))
	if got := len(throttle.lastFired); got > readinessTriggerCap {
		t.Errorf("post-eviction size = %d, must stay ≤ %d", got, readinessTriggerCap)
	}
}

// TestWriteStaleDegraded_PreservesExistingKey — when the cached body already
// has a top-level `degraded` key (e.g. the worker wrote one with a specific
// value), writeStaleDegraded must not splice a second `degraded` field.
// Duplicate keys produce parser-implementation-defined behavior; the worker's
// value is authoritative — pass the body through unchanged (M3).
func TestWriteStaleDegraded_PreservesExistingKey(t *testing.T) {
	original := []byte(`{"refused":false,"score":42,"level":"low","degraded":"compute_pending"}`)

	rr := httptest.NewRecorder()
	writeStaleDegraded(rr, original)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rr.Code)
	}
	got := rr.Body.Bytes()

	// Body must be byte-identical to the input — no splice happened.
	if string(got) != string(original) {
		t.Errorf("body modified:\n got:  %s\n want: %s", got, original)
	}
	// Sanity: there must be exactly one occurrence of `"degraded":` in the
	// response body. A duplicate-key splice would yield two.
	if c := bytes.Count(got, []byte(`"degraded":`)); c != 1 {
		t.Errorf("\"degraded\": occurrences = %d, want 1", c)
	}
}

// TestReadiness_PreferencesVersionInvalidType — cache row carries a
// non-string preferences_version (e.g. numeric, written by a stale/older
// worker). The strict-parse policy (M9) treats this as a mismatch and
// triggers recompute — never silently passes the row through as if its
// version were valid.
func TestReadiness_PreferencesVersionInvalidType(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	// Seed the pref row so DB read succeeds and returns non-nil pref.
	prefTime := time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)
	seedReadinessPref(t, pool, uid, "skadia", prefTime)

	// preferences_version as numeric — violates Spec §5.3 type contract.
	body := json.RawMessage(`{"refused":false,"score":60,"level":"moderate","factors":[],"missing_factors":[],"computed_at":"2026-04-30T10:00:00Z","data_window_end":null,"preferences_version":1,"confidence":null,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(6 * time.Hour) // fresh by expires_at
	seedReadinessRow(t, pool, uid, body, now.Add(-1*time.Hour), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["degraded"] != "service_unavailable" {
		t.Errorf("degraded = %v, want service_unavailable (invalid pref-version type must be treated as mismatch)", got["degraded"])
	}
	if len(fake.Calls) != 1 || fake.Calls[0] != uid {
		t.Errorf("invalidator calls = %v, want [%s] (invalid pref-version must trigger recompute)", fake.Calls, uid)
	}
}

// TestReadiness_PreferencesVersionMatch — cache row has preferences_version
// matching the current pref.updated_at → must serve fresh without degraded.
func TestReadiness_PreferencesVersionMatch(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	// Seed pref with a fixed time.
	prefTime := time.Date(2026, 4, 30, 20, 11, 0, 0, time.UTC)
	seedReadinessPref(t, pool, uid, "manufacturer", prefTime)

	// Cache body carries the same time as preferences_version.
	prefISO := prefTime.UTC().Format(time.RFC3339Nano)
	body := json.RawMessage(`{"refused":false,"score":88,"level":"optimal","factors":[],"missing_factors":[],"computed_at":"2026-04-30T22:00:00Z","data_window_end":null,"preferences_version":"` + prefISO + `","confidence":null,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
	now := time.Now()
	expires := now.Add(6 * time.Hour)
	seedReadinessRow(t, pool, uid, body, now.Add(-30*time.Minute), &expires)

	rr := httptest.NewRecorder()
	h.GetReadiness(rr, reqWithUser(uid))

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No degraded on matching pref version.
	if v, ok := got["degraded"]; ok && v != nil {
		t.Errorf("degraded = %v, want null/absent on pref-version match", v)
	}
	if got["score"].(float64) != 88 {
		t.Errorf("score = %v, want 88", got["score"])
	}
	// No invalidate on a fresh, pref-version-matched cache hit.
	if len(fake.Calls) != 0 {
		t.Errorf("invalidator called %d times, want 0", len(fake.Calls))
	}
}
