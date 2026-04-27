package migrations_test

// Integration tests for migration 000334 (planned_routes_schema_version).
//
// Requires TEST_DATABASE_URL to point to a fully-migrated Skadia database.
// Tests are skipped when the env var is not set so CI without a live DB stays
// green.
//
// Strategy: query information_schema to verify the column exists with the
// expected type and default. The down-migration test applies the DROP inline
// inside a transaction then rolls back, leaving the real schema untouched.

import (
	"context"
	"strings"
	"testing"
)

// TestMigration_PlannedRoutesSchemaVersion_AddsSchemaVersionColumn verifies that
// migration 000334 added route_schema_version as a smallint NOT NULL DEFAULT 0.
func TestMigration_PlannedRoutesSchemaVersion_AddsSchemaVersionColumn(t *testing.T) {
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
		  AND column_name  = 'route_schema_version'
	`).Scan(&dataType, &columnDefault, &isNullable)
	if err != nil {
		t.Fatalf("query information_schema: %v — column may not exist (migration not applied?)", err)
	}

	if !strings.EqualFold(dataType, "smallint") {
		t.Errorf("expected data_type = 'smallint', got %q", dataType)
	}
	if isNullable != "NO" {
		t.Errorf("expected is_nullable = 'NO', got %q", isNullable)
	}
	if !strings.Contains(columnDefault, "0") {
		t.Errorf("expected column_default to contain '0', got %q", columnDefault)
	}
}

// TestMigration_PlannedRoutesSchemaVersion_DownDropsColumn verifies that the
// down-migration SQL successfully drops route_schema_version.
// The DROP is executed inside a transaction that is immediately rolled back so
// the real schema is not modified.
func TestMigration_PlannedRoutesSchemaVersion_DownDropsColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		// Always roll back — we never want to actually drop the column.
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("rollback: %v", err)
		}
	})

	// Apply the down-migration inside the transaction.
	if _, err := tx.Exec(ctx, `ALTER TABLE planned_routes DROP COLUMN route_schema_version`); err != nil {
		t.Fatalf("apply down-migration: %v", err)
	}

	// Verify the column is gone within this transaction.
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name   = 'planned_routes'
		  AND column_name  = 'route_schema_version'
	`).Scan(&count); err != nil {
		t.Fatalf("query information_schema after drop: %v", err)
	}
	if count != 0 {
		t.Errorf("expected column to be absent after down-migration, but information_schema still shows it")
	}
}
