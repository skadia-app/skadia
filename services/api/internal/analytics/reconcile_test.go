//go:build integration

package analytics

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/event"
)

// reconcileTestPool connects to TEST_DATABASE_URL, skipping if unset.
func reconcileTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("reconcileTestPool: connect: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedReconcileUser inserts a test user, sets last_login_at to the given value,
// and registers cleanup for the user row and any analytics_cache rows.
func seedReconcileUser(t *testing.T, pool *pgxpool.Pool, lastLogin time.Time) string {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])

	var uid string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source, last_login_at)
		 VALUES ($1, $2, 'test', $3)
		 RETURNING id`,
		"reconcile-"+suffix,
		"reconcile-"+suffix+"@test.localhost",
		lastLogin,
	).Scan(&uid); err != nil {
		t.Fatalf("seedReconcileUser: insert: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM analytics_cache WHERE user_id = $1`, uid)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, uid)
	})
	return uid
}

// insertFreshCache inserts an analytics_cache row with computed_at = NOW() for
// the given user, simulating a recently computed readiness entry.
func insertFreshCache(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO analytics_cache (user_id, cache_key, data, computed_at, expires_at)
		 VALUES ($1, 'readiness', '{}', NOW(), NOW() + interval '24 hours')
		 ON CONFLICT (user_id, cache_key) DO UPDATE
		   SET data = EXCLUDED.data, computed_at = EXCLUDED.computed_at, expires_at = EXCLUDED.expires_at`,
		userID,
	)
	if err != nil {
		t.Fatalf("insertFreshCache: %v", err)
	}
}

// TestReconcileReadiness_ActiveUserNoCache — active user within 14d, no cache → 1 invalidate.
func TestReconcileReadiness_ActiveUserNoCache(t *testing.T) {
	pool := reconcileTestPool(t)
	uid := seedReconcileUser(t, pool, time.Now().Add(-24*time.Hour)) // 1 day ago

	fake := &event.FakeInvalidator{}
	if err := ReconcileReadiness(context.Background(), pool, fake); err != nil {
		t.Fatalf("ReconcileReadiness returned error: %v", err)
	}

	found := false
	for _, call := range fake.Calls {
		if call == uid {
			found = true
		}
	}
	if !found {
		t.Errorf("expected invalidate for user %s, got calls: %v", uid, fake.Calls)
	}
}

// TestReconcileReadiness_ActiveUserFreshCache — active user with fresh cache → 0 invalidates.
func TestReconcileReadiness_ActiveUserFreshCache(t *testing.T) {
	pool := reconcileTestPool(t)
	uid := seedReconcileUser(t, pool, time.Now().Add(-2*time.Hour)) // 2 hours ago
	insertFreshCache(t, pool, uid)

	fake := &event.FakeInvalidator{}
	if err := ReconcileReadiness(context.Background(), pool, fake); err != nil {
		t.Fatalf("ReconcileReadiness returned error: %v", err)
	}

	for _, call := range fake.Calls {
		if call == uid {
			t.Errorf("unexpected invalidate for user %s with fresh cache", uid)
		}
	}
}

// TestReconcileReadiness_InactiveUser — user last_login_at > 14 days ago → 0 invalidates.
func TestReconcileReadiness_InactiveUser(t *testing.T) {
	pool := reconcileTestPool(t)
	uid := seedReconcileUser(t, pool, time.Now().Add(-30*24*time.Hour)) // 30 days ago

	fake := &event.FakeInvalidator{}
	if err := ReconcileReadiness(context.Background(), pool, fake); err != nil {
		t.Fatalf("ReconcileReadiness returned error: %v", err)
	}

	for _, call := range fake.Calls {
		if call == uid {
			t.Errorf("unexpected invalidate for inactive user %s", uid)
		}
	}
}

// TestReconcileReadiness_LimitRespected — when more stale users than the per-run
// limit exist, only limit many publishes are issued.
func TestReconcileReadiness_LimitRespected(t *testing.T) {
	pool := reconcileTestPool(t)

	// Seed 3 stale users with distinct login times so ORDER BY is deterministic.
	uids := make([]string, 3)
	for i := range uids {
		// oldest first: 10d, 8d, 6d ago
		ago := time.Duration(10-i*2) * 24 * time.Hour
		uids[i] = seedReconcileUser(t, pool, time.Now().Add(-ago))
	}

	// Override env to limit to 2 per run.
	t.Setenv("RECONCILE_MAX_PER_RUN", "2")
	// Use zero delay so the test finishes quickly.
	t.Setenv("RECONCILE_PUBLISH_DELAY_MS", "0")

	fake := &event.FakeInvalidator{}
	if err := ReconcileReadiness(context.Background(), pool, fake); err != nil {
		t.Fatalf("ReconcileReadiness returned error: %v", err)
	}

	// Count how many of our 3 seeded users were published.
	published := 0
	for _, call := range fake.Calls {
		for _, uid := range uids {
			if call == uid {
				published++
			}
		}
	}
	if published > 2 {
		t.Errorf("expected at most 2 publishes for our seeded users, got %d", published)
	}
}

// TestReconcileReadiness_PublishDelay — verifies that the per-publish sleep is
// observed: N users with a known delay must take at least (N-1)*delay.
func TestReconcileReadiness_PublishDelay(t *testing.T) {
	pool := reconcileTestPool(t)

	// Seed 3 stale users.
	for i := 0; i < 3; i++ {
		seedReconcileUser(t, pool, time.Now().Add(-time.Duration(i+1)*24*time.Hour))
	}

	const delayMS = 5
	t.Setenv("RECONCILE_PUBLISH_DELAY_MS", fmt.Sprintf("%d", delayMS))
	t.Setenv("RECONCILE_MAX_PER_RUN", "1000")

	fake := &event.FakeInvalidator{}
	start := time.Now()
	if err := ReconcileReadiness(context.Background(), pool, fake); err != nil {
		t.Fatalf("ReconcileReadiness returned error: %v", err)
	}
	elapsed := time.Since(start)

	// There may be other stale users in the DB from other tests; we only care
	// that at least 2 delays fired (our 3 users → 2 inter-publish sleeps).
	// Use a conservative lower bound to avoid flakiness.
	minExpected := time.Duration(2*delayMS) * time.Millisecond
	if elapsed < minExpected {
		t.Errorf("expected elapsed >= %v (2 delays of %dms), got %v", minExpected, delayMS, elapsed)
	}
}

// TestReconcileReadiness_ContextCancelDuringDelay — cancelling the context while
// the function is sleeping between publishes causes an early return with an error.
func TestReconcileReadiness_ContextCancelDuringDelay(t *testing.T) {
	pool := reconcileTestPool(t)

	// Seed 3 stale users so there are multiple sleeps to cancel.
	for i := 0; i < 3; i++ {
		seedReconcileUser(t, pool, time.Now().Add(-time.Duration(i+1)*24*time.Hour))
	}

	// Long delay ensures the context cancel fires during a sleep.
	t.Setenv("RECONCILE_PUBLISH_DELAY_MS", "5000")
	t.Setenv("RECONCILE_MAX_PER_RUN", "1000")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	fake := &event.FakeInvalidator{}
	start := time.Now()
	err := ReconcileReadiness(ctx, pool, fake)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected error after context cancel, got nil")
	}
	// Must return well before a full 5 s delay cycle.
	if elapsed > 2*time.Second {
		t.Errorf("expected early return after ctx cancel, but elapsed %v", elapsed)
	}
}
