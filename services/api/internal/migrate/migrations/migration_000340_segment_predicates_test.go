package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func readMigration000340SQL(t *testing.T) (upSQL, downSQL string) {
	t.Helper()
	upBytes, err := os.ReadFile("000340_segment_allowed_predicates.up.sql")
	if err != nil {
		t.Fatalf("read 000340 up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000340_segment_allowed_predicates.down.sql")
	if err != nil {
		t.Fatalf("read 000340 down.sql: %v", err)
	}
	return string(upBytes), string(downBytes)
}

func TestMigration000340AddsNullableSegmentPredicates(t *testing.T) {
	upSQL, downSQL := readMigration000340SQL(t)
	for _, needle := range []string{
		"ADD COLUMN IF NOT EXISTS allowed_predicates JSONB DEFAULT NULL",
		"segments_legacy_sub_type_hints",
		"original_sport_types",
		"segments_allowed_predicates_valid",
		"fn_validate_predicates",
		"DROP COLUMN IF EXISTS allowed_predicates",
	} {
		if !strings.Contains(upSQL+downSQL, needle) {
			t.Fatalf("000340 migration missing %q", needle)
		}
	}
}

func TestMigration000340OwnsSharedNormalizerDrop(t *testing.T) {
	up339, _, _ := readMigration000339SQL(t)
	up340, _ := readMigration000340SQL(t)

	if strings.Contains(up339, "DROP FUNCTION IF EXISTS fp_sport_taxonomy_normalize(TEXT, TEXT)") {
		t.Fatal("000339 must leave fp_sport_taxonomy_normalize available for 000340")
	}
	if !strings.Contains(up340, "fp_sport_taxonomy_normalize") {
		t.Fatal("000340 must use fp_sport_taxonomy_normalize for legacy segment hint backfill")
	}
	if !strings.Contains(up340, "DROP FUNCTION IF EXISTS fp_sport_taxonomy_normalize(TEXT, TEXT)") {
		t.Fatal("000340 must drop fp_sport_taxonomy_normalize after the final migration that uses it")
	}
}

func TestMigration000340BackfillsTuplePredicates(t *testing.T) {
	upSQL, _ := readMigration000340SQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 340, "000340 tuple backfill needs pre-000340 segment predicate columns")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.allow_segment_sport_broadening = 'on'`); err != nil {
		t.Fatalf("set gate: %v", err)
	}
	up339, _, _ := readMigration000339SQL(t)
	if _, err := tx.Exec(ctx, up339); err != nil {
		t.Fatalf("apply 000339: %v", err)
	}

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, is_admin)
		VALUES ($1, $2, $3, false)
	`, userID, "mig340_"+userID[:8], "mig340_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	segmentID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, allowed_sport_types, allowed_sub_types)
		VALUES ($1, $2, 'tuple segment', ARRAY['cycling','running'], ARRAY['mountain','trail'])
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	noneID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, allowed_sport_types, allowed_sub_types)
		VALUES ($1, $2, 'none segment', ARRAY[]::text[], ARRAY[]::text[])
	`, noneID, userID); err != nil {
		t.Fatalf("insert empty segment: %v", err)
	}
	allID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, allowed_sport_types, allowed_sub_types)
		VALUES ($1, $2, 'all segment', NULL, NULL)
	`, allID, userID); err != nil {
		t.Fatalf("insert all segment: %v", err)
	}

	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply 000340: %v", err)
	}

	var tuplePreds, nonePreds string
	var allPreds *string
	if err := tx.QueryRow(ctx, `SELECT allowed_predicates::text FROM segments WHERE id = $1`, segmentID).Scan(&tuplePreds); err != nil {
		t.Fatalf("query tuple predicates: %v", err)
	}
	if !strings.Contains(tuplePreds, `"sport": "cycling"`) || !strings.Contains(tuplePreds, `"sub_type": "mountain"`) || !strings.Contains(tuplePreds, `"sport": "running"`) || !strings.Contains(tuplePreds, `"sub_type": "trail"`) {
		t.Fatalf("tuple predicates = %s", tuplePreds)
	}
	var invalidPairs int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM segments s
		CROSS JOIN LATERAL jsonb_array_elements(s.allowed_predicates) pred
		WHERE s.id = $1
		  AND (
		    (pred->>'sport' = 'running' AND pred->>'sub_type' = 'mountain')
		    OR (pred->>'sport' = 'cycling' AND pred->>'sub_type' = 'trail')
		  )
	`, segmentID).Scan(&invalidPairs); err != nil {
		t.Fatalf("query invalid pairs: %v", err)
	}
	if invalidPairs != 0 {
		t.Fatalf("tuple predicates contains %d cross-product pairs: %s", invalidPairs, tuplePreds)
	}
	if err := tx.QueryRow(ctx, `SELECT allowed_predicates::text FROM segments WHERE id = $1`, noneID).Scan(&nonePreds); err != nil {
		t.Fatalf("query empty predicates: %v", err)
	}
	if nonePreds != "[]" {
		t.Fatalf("empty allowed_predicates = %s, want []", nonePreds)
	}
	if err := tx.QueryRow(ctx, `SELECT allowed_predicates::text FROM segments WHERE id = $1`, allID).Scan(&allPreds); err != nil {
		t.Fatalf("query all predicates: %v", err)
	}
	if allPreds != nil {
		t.Fatalf("all allowed_predicates = %v, want NULL", *allPreds)
	}
}

func TestMigration000340BackfillsLegacyHintsBeforeCollapsedSports(t *testing.T) {
	upSQL, _ := readMigration000340SQL(t)
	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 340, "000340 legacy hint backfill needs pre-000340 segment predicate columns")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, `SET LOCAL skadia.allow_segment_sport_broadening = 'on'`); err != nil {
		t.Fatalf("set gate: %v", err)
	}

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, is_admin)
		VALUES ($1, $2, $3, false)
	`, userID, "mig340legacy_"+userID[:8], "mig340legacy_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	segmentID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, allowed_sport_types, allowed_sub_types)
		VALUES ($1, $2, 'legacy mtb segment', ARRAY['mountain_biking'], NULL)
	`, segmentID, userID); err != nil {
		t.Fatalf("insert legacy segment: %v", err)
	}

	up339, _, _ := readMigration000339SQL(t)
	if _, err := tx.Exec(ctx, up339); err != nil {
		t.Fatalf("apply 000339: %v", err)
	}
	if _, err := tx.Exec(ctx, upSQL); err != nil {
		t.Fatalf("apply 000340: %v", err)
	}

	var predicates string
	if err := tx.QueryRow(ctx, `SELECT allowed_predicates::text FROM segments WHERE id = $1`, segmentID).Scan(&predicates); err != nil {
		t.Fatalf("query predicates: %v", err)
	}
	if !strings.Contains(predicates, `"sport": "cycling"`) || !strings.Contains(predicates, `"sub_type": "mountain"`) {
		t.Fatalf("legacy predicates = %s, want cycling/mountain from hint table", predicates)
	}
	if strings.Contains(predicates, `{"sport": "cycling"}`) || !strings.Contains(predicates, `"sub_type"`) {
		t.Fatalf("legacy predicates = %s, must not broaden to plain cycling", predicates)
	}
}
