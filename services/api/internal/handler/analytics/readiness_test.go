//go:build integration

package analytics

import (
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

	body := json.RawMessage(`{"refused":false,"score":72.5,"level":"green","factors":[],"missing_factors":[],"computed_at":"2026-04-30T12:00:00Z","data_window_end":null,"preferences_version":1,"confidence":0.8,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
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
	if got["score"].(float64) != 72.5 {
		t.Errorf("score = %v, want 72.5", got["score"])
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

// TestReadiness_Miss — no row → refused envelope with reason=no_data.
func TestReadiness_Miss(t *testing.T) {
	h, pool := newReadinessTestHandler(t, &event.FakeInvalidator{})
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
	if got["refusal_reason"] != "no_data" {
		t.Errorf("refusal_reason = %v, want no_data", got["refusal_reason"])
	}
	// Schema-completeness: required null-able fields must be present.
	for _, k := range []string{"score", "level", "factors", "missing_factors", "computed_at", "data_window_end", "preferences_version", "confidence", "recommendation_code", "signal_modifier", "degraded"} {
		if _, ok := got[k]; !ok {
			t.Errorf("refused envelope missing field %q", k)
		}
	}
}

// TestReadiness_StaleInvalidatorOK — stale cache + healthy invalidator returns
// the stale body unchanged AND records the publish call.
func TestReadiness_StaleInvalidatorOK(t *testing.T) {
	fake := &event.FakeInvalidator{}
	h, pool := newReadinessTestHandler(t, fake)
	uid := seedReadinessTestUser(t, pool)

	body := json.RawMessage(`{"refused":false,"score":50,"level":"yellow","factors":[],"missing_factors":[],"computed_at":"2026-04-30T10:00:00Z","data_window_end":null,"preferences_version":1,"confidence":0.7,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
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

	body := json.RawMessage(`{"refused":false,"score":40,"level":"yellow","factors":[],"missing_factors":[],"computed_at":"2026-04-30T10:00:00Z","data_window_end":null,"preferences_version":1,"confidence":0.6,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
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

	body := json.RawMessage(`{"refused":false,"score":33,"level":"yellow","factors":[],"missing_factors":[],"computed_at":"2026-04-30T09:00:00Z","data_window_end":null,"preferences_version":1,"confidence":0.5,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
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

	body := json.RawMessage(`{"refused":false,"score":80,"level":"green","factors":[],"missing_factors":[],"computed_at":"2026-04-28T10:00:00Z","data_window_end":null,"preferences_version":1,"confidence":0.9,"recommendation_code":null,"signal_modifier":null,"degraded":null}`)
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
