package migrations_test

// Integration tests for migration 000343 (user_readiness_preferences).
//
// Requires TEST_DATABASE_URL to point to a fully-migrated Skadia database.
// Tests are skipped when the env var is not set so CI without a live DB stays
// green.
//
// Strategy: each test runs the up-migration SQL inside a transaction that is
// rolled back at the end so the real schema stays untouched.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// migration000343UpSQL loads the real migration file at test runtime so the
// test always reflects the actual SQL on disk (not a stale inline copy).
func migration000343UpSQL(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("000343_user_readiness_preferences.up.sql")
	if err != nil {
		t.Fatalf("read 000343 up.sql: %v", err)
	}
	return string(b)
}

const migration000343DownSQL = `DROP TABLE IF EXISTS user_readiness_preferences;`

// TestMigration000343_TableCreated verifies that information_schema.tables
// contains user_readiness_preferences after running the up-migration.
func TestMigration000343_TableCreated(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	if _, err := tx.Exec(ctx, migration000343UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name   = 'user_readiness_preferences'
	`).Scan(&count); err != nil {
		t.Fatalf("query information_schema.tables: %v", err)
	}
	if count != 1 {
		t.Errorf("expected user_readiness_preferences in information_schema.tables, got count=%d", count)
	}
}

// TestMigration000343_CheckConstraintMentionsBothSources verifies that the
// CHECK constraint definition contains both 'manufacturer' and 'skadia'.
func TestMigration000343_CheckConstraintMentionsBothSources(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	if _, err := tx.Exec(ctx, migration000343UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var def string
	err = tx.QueryRow(ctx, `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		WHERE t.relname = 'user_readiness_preferences'
		  AND c.contype = 'c'
		LIMIT 1
	`).Scan(&def)
	if err != nil {
		t.Fatalf("query pg_get_constraintdef: %v", err)
	}
	if !strings.Contains(def, "'manufacturer'") {
		t.Errorf("expected CHECK constraint to contain 'manufacturer', got: %s", def)
	}
	if !strings.Contains(def, "'skadia'") {
		t.Errorf("expected CHECK constraint to contain 'skadia', got: %s", def)
	}
}

// TestMigration000343_AnalyticsCacheCleanup verifies that a synthetic
// analytics_cache row with cache_key='readiness_v2' inserted before the
// up-migration is removed by the DELETE in the migration.
func TestMigration000343_AnalyticsCacheCleanup(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// Insert a real user (analytics_cache.user_id has FK→users.id).
	userID := uuid.NewString()
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"mig343_cache_"+userID[:8],
		"mig343_cache_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	// Insert synthetic readiness_v2 cache row.
	_, err = tx.Exec(ctx,
		`INSERT INTO analytics_cache (user_id, cache_key, data) VALUES ($1, 'readiness_v2', '{"score":50}'::jsonb)`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert analytics_cache row: %v", err)
	}

	// Verify it's there before.
	var before int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM analytics_cache WHERE user_id = $1 AND cache_key = 'readiness_v2'`,
		userID,
	).Scan(&before); err != nil {
		t.Fatalf("query before: %v", err)
	}
	if before != 1 {
		t.Fatalf("expected synthetic row to exist before migration, got count=%d", before)
	}

	// Apply migration.
	if _, err := tx.Exec(ctx, migration000343UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var after int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM analytics_cache WHERE user_id = $1 AND cache_key = 'readiness_v2'`,
		userID,
	).Scan(&after); err != nil {
		t.Fatalf("query after: %v", err)
	}
	if after != 0 {
		t.Errorf("expected readiness_v2 cache row to be removed by migration, got count=%d", after)
	}
}

// TestMigration000343_AnalyticsCacheCleanupLegacy verifies that the second
// DELETE in the up-migration removes legacy analytics_cache rows written with
// cache_key='readiness' (the old shape before the _v2 rename).
func TestMigration000343_AnalyticsCacheCleanupLegacy(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// Insert a real user (analytics_cache.user_id has FK→users.id).
	userID := uuid.NewString()
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"mig343_leg_"+userID[:8],
		"mig343_leg_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	// Insert synthetic legacy readiness cache row (old shape, no _v2 suffix).
	_, err = tx.Exec(ctx,
		`INSERT INTO analytics_cache (user_id, cache_key, data) VALUES ($1, 'readiness', '{"score":42,"level":"good","factors":[]}'::jsonb)`,
		userID,
	)
	if err != nil {
		t.Fatalf("insert analytics_cache legacy row: %v", err)
	}

	// Verify it's there before.
	var before int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM analytics_cache WHERE user_id = $1 AND cache_key = 'readiness'`,
		userID,
	).Scan(&before); err != nil {
		t.Fatalf("query before: %v", err)
	}
	if before != 1 {
		t.Fatalf("expected legacy row to exist before migration, got count=%d", before)
	}

	// Apply migration.
	if _, err := tx.Exec(ctx, migration000343UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var after int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM analytics_cache WHERE user_id = $1 AND cache_key = 'readiness'`,
		userID,
	).Scan(&after); err != nil {
		t.Fatalf("query after: %v", err)
	}
	if after != 0 {
		t.Errorf("expected legacy 'readiness' cache row to be removed by migration, got count=%d", after)
	}
}

// TestMigration000343_AuditLogRow verifies that the up-migration inserts one
// row into auth_audit_log with migration='000343'.
func TestMigration000343_AuditLogRow(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// Count existing rows before so we check the delta (DB may already have rows).
	var before int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM auth_audit_log
		WHERE event_type = 'migration'
		  AND details->>'migration' = '000343'
	`).Scan(&before); err != nil {
		t.Fatalf("query auth_audit_log before: %v", err)
	}

	if _, err := tx.Exec(ctx, migration000343UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var after int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM auth_audit_log
		WHERE event_type = 'migration'
		  AND details->>'migration' = '000343'
	`).Scan(&after); err != nil {
		t.Fatalf("query auth_audit_log after: %v", err)
	}
	if after != before+1 {
		t.Errorf("expected exactly one new auth_audit_log row for migration 000343, got before=%d after=%d", before, after)
	}
}

// TestMigration000343_DownSmoke verifies the up→insert→down→up cycle:
// up-migration creates the table, an inserted test row exists, down-migration
// drops the table, second up-migration recreates it empty.
func TestMigration000343_DownSmoke(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// 1) Up.
	if _, err := tx.Exec(ctx, migration000343UpSQL(t)); err != nil {
		t.Fatalf("first up: %v", err)
	}

	// Insert test user + readiness preference row.
	userID := uuid.NewString()
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"mig343_down_"+userID[:8],
		"mig343_down_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_readiness_preferences (user_id, preferred_source) VALUES ($1, 'manufacturer')`,
		userID,
	); err != nil {
		t.Fatalf("insert preference row: %v", err)
	}

	// 2) Down.
	if _, err := tx.Exec(ctx, migration000343DownSQL); err != nil {
		t.Fatalf("apply down: %v", err)
	}

	var present int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name   = 'user_readiness_preferences'
	`).Scan(&present); err != nil {
		t.Fatalf("query after down: %v", err)
	}
	if present != 0 {
		t.Errorf("expected table to be absent after down-migration, got count=%d", present)
	}

	// 3) Up again — table back, empty.
	if _, err := tx.Exec(ctx, migration000343UpSQL(t)); err != nil {
		t.Fatalf("second up: %v", err)
	}

	var rowCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_readiness_preferences`).Scan(&rowCount); err != nil {
		t.Fatalf("query row count after second up: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("expected user_readiness_preferences to be empty after re-creation, got count=%d", rowCount)
	}
}
