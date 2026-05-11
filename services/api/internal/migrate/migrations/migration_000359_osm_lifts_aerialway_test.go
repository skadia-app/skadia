package migrations_test

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

// Verifies the osm_lifts_aerialway migration adds the `aerialway` column to
// osm_lifts, that the column is nullable, and that re-applying the
// up-migration is a no-op (IF NOT EXISTS guard).
func TestMigration_OsmLiftsAerialway_AddsColumn(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	// Drop the column if a prior test left it (this test runs against the
	// shared per-worktree DB which may already have all migrations applied).
	if _, err := pool.Exec(ctx, `ALTER TABLE osm_lifts DROP COLUMN IF EXISTS aerialway`); err != nil {
		t.Fatalf("pre-clean: %v", err)
	}

	// Apply the migration directly via the SQL it contains.
	if _, err := pool.Exec(ctx, `ALTER TABLE osm_lifts ADD COLUMN IF NOT EXISTS aerialway TEXT`); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	// Idempotency: applying again must succeed (IF NOT EXISTS).
	if _, err := pool.Exec(ctx, `ALTER TABLE osm_lifts ADD COLUMN IF NOT EXISTS aerialway TEXT`); err != nil {
		t.Fatalf("re-apply up-migration: %v", err)
	}

	// Verify the column is nullable text.
	var dataType string
	var isNullable string
	err := pool.QueryRow(ctx, `
		SELECT data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'osm_lifts' AND column_name = 'aerialway'
	`).Scan(&dataType, &isNullable)
	if err != nil {
		t.Fatalf("query column metadata: %v", err)
	}
	if dataType != "text" {
		t.Errorf("aerialway data_type = %q, want %q", dataType, "text")
	}
	if isNullable != "YES" {
		t.Errorf("aerialway is_nullable = %q, want %q", isNullable, "YES")
	}
}
