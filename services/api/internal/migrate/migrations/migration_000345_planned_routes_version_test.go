package migrations_test

// Integration tests for migration 000345 (planned_routes.version).
//
// Originally written as a 000999 placeholder; finalize-migration.sh renumbered
// the file to 000345 at merge time. The test name stays attached to the
// migration regardless of its final number.
//
// Requires TEST_DATABASE_URL to point to a fully-migrated Skadia database.
// Tests are skipped when the env var is not set.

import (
	"context"
	"strings"
	"testing"
)

// TestMigration_PlannedRoutesVersion_AddsVersionColumn verifies that
// migration 000345 added `version` as a BIGINT NOT NULL DEFAULT 1.
//
// The default is 1 (not 0) because the column doubles as the strong-ETag
// source: every legacy row must already publish a non-zero version on the
// first GET so clients have a usable If-Match value before any write.
func TestMigration_PlannedRoutesVersion_AddsVersionColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var dataType string
	var columnDefault string
	var isNullable string

	err := pool.QueryRow(ctx, `
		SELECT data_type, column_default, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name   = 'planned_routes'
		  AND column_name  = 'version'
	`).Scan(&dataType, &columnDefault, &isNullable)
	if err != nil {
		t.Fatalf("query information_schema: %v — column may not exist (migration not applied?)", err)
	}

	if !strings.EqualFold(dataType, "bigint") {
		t.Errorf("expected data_type = 'bigint', got %q", dataType)
	}
	if isNullable != "NO" {
		t.Errorf("expected is_nullable = 'NO', got %q", isNullable)
	}
	if !strings.Contains(columnDefault, "1") {
		t.Errorf("expected column_default to contain '1', got %q", columnDefault)
	}
}

// TestMigration_PlannedRoutesVersion_DownDropsColumn verifies the down
// migration removes `version`. Executed inside a rolled-back transaction.
func TestMigration_PlannedRoutesVersion_DownDropsColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("rollback: %v", err)
		}
	})

	if _, err := tx.Exec(ctx, `ALTER TABLE planned_routes DROP COLUMN version`); err != nil {
		t.Fatalf("apply down-migration: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name   = 'planned_routes'
		  AND column_name  = 'version'
	`).Scan(&count); err != nil {
		t.Fatalf("query information_schema after drop: %v", err)
	}
	if count != 0 {
		t.Errorf("expected column to be absent after down-migration, got %d", count)
	}
}
