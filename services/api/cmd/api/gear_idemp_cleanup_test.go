package main

// Smoke tests for gearIdempotencyCleanupEnabled (Re-Critic #15).
//
// Tests use the per-worktree test DB (TEST_DATABASE_URL via scripts/test_db.sh).
// They are skipped automatically when TEST_DATABASE_URL is unset.

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

// TestGearIdempotencyCleanupEnabled_NoPgCron verifies that
// gearIdempotencyCleanupEnabled returns true when the test DB has no pg_cron
// extension (the expected state for dev/test databases).
func TestGearIdempotencyCleanupEnabled_NoPgCron(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	// Detect actual pg_cron state and assert gearIdempotencyCleanupEnabled
	// returns the inverse (i.e. enable cleanup when pg_cron absent).
	var hasCron bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='pg_cron')`).Scan(&hasCron); err != nil {
		t.Fatalf("detect pg_cron: %v", err)
	}

	enabled := gearIdempotencyCleanupEnabled(ctx, pool)
	if enabled == hasCron {
		t.Errorf("gearIdempotencyCleanupEnabled=%v but pg_cron present=%v; want inverse", enabled, hasCron)
	}
}
