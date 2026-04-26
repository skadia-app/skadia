package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMigration000328UsesCategoryLevelLegacyBypass(t *testing.T) {
	migrationSQLBytes, err := os.ReadFile("000328_drop_activity_surfaces.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	migrationSQL := string(migrationSQLBytes)

	if strings.Contains(migrationSQL, "ON CONFLICT") {
		t.Fatal("expected migration to bypass legacy copy per activity/category, not per tuple conflict")
	}
	if !strings.Contains(migrationSQL, "existing.activity_id = grouped.activity_id") ||
		!strings.Contains(migrationSQL, "existing.category = grouped.category") {
		t.Fatal("expected migration to skip legacy rows when an activity/category already has enrichment rows")
	}
}

func TestMigration000328BackfillsLegacyActivitySurfaces(t *testing.T) {
	migrationSQL, err := os.ReadFile("000328_drop_activity_surfaces.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Logf("rollback: %v", err)
		}
	})

	userID := uuid.NewString()
	activityID := uuid.NewString()
	activityWithReplacementSurfaceID := uuid.NewString()
	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, userID, "mig328_"+userID[:8], "mig328_"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, started_at, created_at)
		VALUES ($1, $2, 'migration 328 test', 'cycling', NOW(), NOW())
	`, activityID, userID)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, started_at, created_at)
		VALUES ($1, $2, 'migration 328 category skip test', 'cycling', NOW(), NOW())
	`, activityWithReplacementSurfaceID, userID)
	if err != nil {
		t.Fatalf("insert replacement activity: %v", err)
	}

	_, err = tx.Exec(ctx, `
		DROP TABLE IF EXISTS activity_surfaces;
		CREATE TABLE activity_surfaces (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
			surface_type TEXT NOT NULL,
			path_type TEXT NOT NULL DEFAULT 'unknown',
			length_m DOUBLE PRECISION,
			percentage DOUBLE PRECISION,
			category TEXT NOT NULL DEFAULT 'surface'
		);
		CREATE INDEX idx_activity_surfaces_activity ON activity_surfaces (activity_id);

		INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m)
		VALUES ($2, 'surface', 'gravel', 77, 77);

		INSERT INTO activity_surfaces (activity_id, surface_type, path_type, length_m, percentage, category)
		VALUES
			($1, 'asphalt', 'cycleway', 40, 30, 'surface'),
			($1, 'asphalt', 'path', 60, 80, 'surface'),
			($1, 'gravel', 'track', 12, 12, 'surface'),
			($1, '', 'residential', NULL, NULL, 'highway'),
			($1, 'dirt', 'track', 8, 8, 'highway'),
			($1, '', '', NULL, NULL, ''),
			($2, 'asphalt', 'cycleway', 40, 40, 'surface'),
			($2, '', 'residential', 60, 60, 'highway');
	`, activityID, activityWithReplacementSurfaceID)
	if err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	if _, err := tx.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	var surfaceAsphaltLength, surfaceAsphaltPct float64
	if err := tx.QueryRow(ctx, `
		SELECT length_m, percentage
		FROM activity_enrichments
		WHERE activity_id = $1 AND category = 'surface' AND type = 'asphalt'
	`, activityID).Scan(&surfaceAsphaltLength, &surfaceAsphaltPct); err != nil {
		t.Fatalf("query asphalt enrichment: %v", err)
	}
	if surfaceAsphaltLength != 100 {
		t.Fatalf("expected asphalt length 100, got %v", surfaceAsphaltLength)
	}
	if surfaceAsphaltPct != 100 {
		t.Fatalf("expected asphalt percentage clamped to 100, got %v", surfaceAsphaltPct)
	}

	var legacyGravelLength, legacyGravelPct float64
	if err := tx.QueryRow(ctx, `
		SELECT length_m, percentage
		FROM activity_enrichments
		WHERE activity_id = $1 AND category = 'surface' AND type = 'gravel'
	`, activityID).Scan(&legacyGravelLength, &legacyGravelPct); err != nil {
		t.Fatalf("query legacy gravel enrichment: %v", err)
	}
	if legacyGravelLength != 12 || legacyGravelPct != 12 {
		t.Fatalf("expected legacy gravel enrichment to backfill as 12/12, got %v/%v", legacyGravelLength, legacyGravelPct)
	}

	var highwayResidentialLength, highwayResidentialPct float64
	if err := tx.QueryRow(ctx, `
		SELECT length_m, percentage
		FROM activity_enrichments
		WHERE activity_id = $1 AND category = 'highway' AND type = 'residential'
	`, activityID).Scan(&highwayResidentialLength, &highwayResidentialPct); err != nil {
		t.Fatalf("query highway enrichment: %v", err)
	}
	if highwayResidentialLength != 0 || highwayResidentialPct != 0 {
		t.Fatalf("expected null legacy values to backfill as 0/0, got %v/%v", highwayResidentialLength, highwayResidentialPct)
	}

	var unknownLength, unknownPct float64
	if err := tx.QueryRow(ctx, `
		SELECT length_m, percentage
		FROM activity_enrichments
		WHERE activity_id = $1 AND category = 'surface' AND type = 'unknown'
	`, activityID).Scan(&unknownLength, &unknownPct); err != nil {
		t.Fatalf("query unknown enrichment: %v", err)
	}
	if unknownLength != 0 || unknownPct != 0 {
		t.Fatalf("expected empty legacy type to backfill as unknown 0/0, got %v/%v", unknownLength, unknownPct)
	}

	var skippedSurfaceCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM activity_enrichments
		WHERE activity_id = $1 AND category = 'surface'
	`, activityWithReplacementSurfaceID).Scan(&skippedSurfaceCount); err != nil {
		t.Fatalf("query replacement activity surface count: %v", err)
	}
	if skippedSurfaceCount != 1 {
		t.Fatalf("expected replacement activity surface legacy rows to be skipped, got %d rows", skippedSurfaceCount)
	}

	var replacementGravelLength, replacementGravelPct float64
	if err := tx.QueryRow(ctx, `
		SELECT length_m, percentage
		FROM activity_enrichments
		WHERE activity_id = $1 AND category = 'surface' AND type = 'gravel'
	`, activityWithReplacementSurfaceID).Scan(&replacementGravelLength, &replacementGravelPct); err != nil {
		t.Fatalf("query replacement gravel enrichment: %v", err)
	}
	if replacementGravelLength != 77 || replacementGravelPct != 77 {
		t.Fatalf("expected existing replacement gravel enrichment to remain 77/77, got %v/%v", replacementGravelLength, replacementGravelPct)
	}

	var copiedHighwayLength, copiedHighwayPct float64
	if err := tx.QueryRow(ctx, `
		SELECT length_m, percentage
		FROM activity_enrichments
		WHERE activity_id = $1 AND category = 'highway' AND type = 'residential'
	`, activityWithReplacementSurfaceID).Scan(&copiedHighwayLength, &copiedHighwayPct); err != nil {
		t.Fatalf("query replacement activity highway enrichment: %v", err)
	}
	if copiedHighwayLength != 60 || copiedHighwayPct != 60 {
		t.Fatalf("expected highway category without replacement rows to backfill, got %v/%v", copiedHighwayLength, copiedHighwayPct)
	}

	var legacyTableExists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('activity_surfaces') IS NOT NULL`).Scan(&legacyTableExists); err != nil {
		t.Fatalf("query legacy table existence: %v", err)
	}
	if legacyTableExists {
		t.Fatal("expected activity_surfaces to be dropped")
	}
}
