package migrations_test

// Integration tests for migration 000999 (user_readiness_preferences).
//
// Requires TEST_DATABASE_URL to point to a fully-migrated Skadia database.
// Tests are skipped when the env var is not set so CI without a live DB stays
// green.
//
// Strategy: each test runs the up-migration SQL inside a transaction that is
// rolled back at the end so the real schema stays untouched. This works even
// before the migration has been applied to the test database (the placeholder
// number 000999 will be renumbered at merge time).

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const migration000999UpSQL = `
CREATE TABLE IF NOT EXISTS user_readiness_preferences (
    user_id          UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    preferred_source TEXT
        CHECK (preferred_source IS NULL OR preferred_source IN ('manufacturer','skadia')),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DELETE FROM analytics_cache WHERE cache_key = 'readiness_v2';
`

const migration000999DownSQL = `DROP TABLE IF EXISTS user_readiness_preferences;`

// TestMigration000999_TableCreated verifies that information_schema.tables
// contains user_readiness_preferences after running the up-migration.
func TestMigration000999_TableCreated(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	if _, err := tx.Exec(ctx, migration000999UpSQL); err != nil {
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

// TestMigration000999_CheckConstraintMentionsBothSources verifies that the
// CHECK constraint definition contains both 'manufacturer' and 'skadia'.
func TestMigration000999_CheckConstraintMentionsBothSources(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	if _, err := tx.Exec(ctx, migration000999UpSQL); err != nil {
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

// TestMigration000999_AnalyticsCacheCleanup verifies that a synthetic
// analytics_cache row with cache_key='readiness_v2' inserted before the
// up-migration is removed by the DELETE in the migration.
func TestMigration000999_AnalyticsCacheCleanup(t *testing.T) {
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
		"mig999_cache_"+userID[:8],
		"mig999_cache_"+userID[:8]+"@example.com",
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
	if _, err := tx.Exec(ctx, migration000999UpSQL); err != nil {
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

// TestMigration000999_DownSmoke verifies the up→insert→down→up cycle:
// up-migration creates the table, an inserted test row exists, down-migration
// drops the table, second up-migration recreates it empty.
func TestMigration000999_DownSmoke(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// 1) Up.
	if _, err := tx.Exec(ctx, migration000999UpSQL); err != nil {
		t.Fatalf("first up: %v", err)
	}

	// Insert test user + readiness preference row.
	userID := uuid.NewString()
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"mig999_down_"+userID[:8],
		"mig999_down_"+userID[:8]+"@example.com",
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
	if _, err := tx.Exec(ctx, migration000999DownSQL); err != nil {
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
	if _, err := tx.Exec(ctx, migration000999UpSQL); err != nil {
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
