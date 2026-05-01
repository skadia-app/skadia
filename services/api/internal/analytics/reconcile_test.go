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
