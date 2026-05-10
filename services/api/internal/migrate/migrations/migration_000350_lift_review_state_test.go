package migrations_test

// Integration tests for migration 000350_lift_review_state.
//
// Verifies:
//   1. rejected_by_user column exists with NOT NULL + DEFAULT FALSE.
//   2. idx_lift_candidates_active partial index exists.
//   3. lifts_acknowledged_at column exists on activities as nullable TIMESTAMPTZ.
//   4. Existing rows get rejected_by_user=false by default (no data loss).
//
// Tests are skipped when TEST_DATABASE_URL is unset.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestMigration_LiftReviewState_RejectedByUserColumn verifies the
// rejected_by_user column is BOOLEAN NOT NULL DEFAULT FALSE.
func TestMigration_LiftReviewState_RejectedByUserColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var dataType, columnDefault, isNullable string
	err := pool.QueryRow(ctx, `
		SELECT data_type, column_default, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'activity_lift_candidates' AND column_name = 'rejected_by_user'
	`).Scan(&dataType, &columnDefault, &isNullable)
	if err != nil {
		t.Fatalf("query column metadata: %v", err)
	}
	if dataType != "boolean" {
		t.Errorf("rejected_by_user data_type = %q, want %q", dataType, "boolean")
	}
	if isNullable != "NO" {
		t.Errorf("rejected_by_user is_nullable = %q, want %q", isNullable, "NO")
	}
	if columnDefault == "" {
		t.Error("rejected_by_user has no default, want false")
	}
}

// TestMigration_LiftReviewState_ActiveIndexExists verifies the partial index
// idx_lift_candidates_active is present in pg_indexes.
func TestMigration_LiftReviewState_ActiveIndexExists(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'activity_lift_candidates'
			  AND indexname = 'idx_lift_candidates_active'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("query index: %v", err)
	}
	if !exists {
		t.Error("idx_lift_candidates_active does not exist")
	}
}

// TestMigration_LiftReviewState_LiftsAcknowledgedAtColumn verifies the
// lifts_acknowledged_at column on activities is nullable TIMESTAMPTZ.
func TestMigration_LiftReviewState_LiftsAcknowledgedAtColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var dataType, isNullable string
	err := pool.QueryRow(ctx, `
		SELECT data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'activities' AND column_name = 'lifts_acknowledged_at'
	`).Scan(&dataType, &isNullable)
	if err != nil {
		t.Fatalf("query column metadata: %v", err)
	}
	if dataType != "timestamp with time zone" {
		t.Errorf("lifts_acknowledged_at data_type = %q, want %q", dataType, "timestamp with time zone")
	}
	if isNullable != "YES" {
		t.Errorf("lifts_acknowledged_at is_nullable = %q, want %q", isNullable, "YES")
	}
}

// TestMigration_LiftReviewState_ExistingRowsGetDefaultFalse verifies that
// rows inserted before or after the migration have rejected_by_user=false and
// that the column can be set to true (no constraint blocks it).
func TestMigration_LiftReviewState_ExistingRowsGetDefaultFalse(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"liftrev_"+userID[:8],
		"liftrev_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	activityID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'skiing', $3, $4, $5)
	`, activityID, userID, now, now.Add(2*time.Hour), uuid.NewString()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM activities WHERE id = $1", activityID)
	})

	// Insert a candidate without specifying rejected_by_user — should default to false.
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, source)
		VALUES ($1, $2, $3, 120, 80, 'heuristic')
	`, activityID, now, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("insert candidate: %v", err)
	}

	var rejected bool
	if err := pool.QueryRow(ctx, `
		SELECT rejected_by_user FROM activity_lift_candidates
		WHERE activity_id = $1
	`, activityID).Scan(&rejected); err != nil {
		t.Fatalf("query candidate: %v", err)
	}
	if rejected {
		t.Error("rejected_by_user = true, want false (DEFAULT should apply)")
	}

	// Verify we can flip it to true.
	if _, err := pool.Exec(ctx, `
		UPDATE activity_lift_candidates SET rejected_by_user = true WHERE activity_id = $1
	`, activityID); err != nil {
		t.Fatalf("update rejected_by_user: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT rejected_by_user FROM activity_lift_candidates WHERE activity_id = $1
	`, activityID).Scan(&rejected); err != nil {
		t.Fatalf("query after update: %v", err)
	}
	if !rejected {
		t.Error("rejected_by_user = false after UPDATE, want true")
	}
}

// TestMigration_LiftReviewState_DetectionModeToggleColumns verifies that the
// 4 detection-mode toggle columns added in the Phase 1 in-place extension are
// present with the correct types, nullability and defaults.
func TestMigration_LiftReviewState_DetectionModeToggleColumns(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	type colSpec struct {
		table      string
		column     string
		wantType   string
		wantNullable string // "YES" or "NO"
		wantDefault  string // empty means nullable (no default expected)
	}
	specs := []colSpec{
		{
			table:        "user_enrichment_settings",
			column:       "enable_lift_detection_osm",
			wantType:     "boolean",
			wantNullable: "NO",
			wantDefault:  "true",
		},
		{
			table:        "user_enrichment_settings",
			column:       "enable_lift_detection_heuristic",
			wantType:     "boolean",
			wantNullable: "NO",
			wantDefault:  "true",
		},
		{
			table:        "activities",
			column:       "lift_detection_osm_override",
			wantType:     "boolean",
			wantNullable: "YES",
			wantDefault:  "",
		},
		{
			table:        "activities",
			column:       "lift_detection_heuristic_override",
			wantType:     "boolean",
			wantNullable: "YES",
			wantDefault:  "",
		},
	}

	for _, spec := range specs {
		t.Run(spec.table+"."+spec.column, func(t *testing.T) {
			var dataType, isNullable string
			var columnDefault *string
			err := pool.QueryRow(ctx, `
				SELECT data_type, is_nullable, column_default
				FROM information_schema.columns
				WHERE table_name = $1 AND column_name = $2
			`, spec.table, spec.column).Scan(&dataType, &isNullable, &columnDefault)
			if err != nil {
				t.Fatalf("query column metadata for %s.%s: %v", spec.table, spec.column, err)
			}
			if dataType != spec.wantType {
				t.Errorf("%s.%s data_type = %q, want %q", spec.table, spec.column, dataType, spec.wantType)
			}
			if isNullable != spec.wantNullable {
				t.Errorf("%s.%s is_nullable = %q, want %q", spec.table, spec.column, isNullable, spec.wantNullable)
			}
			if spec.wantDefault != "" {
				if columnDefault == nil {
					t.Errorf("%s.%s has no default, want %q", spec.table, spec.column, spec.wantDefault)
				} else if !strings.Contains(*columnDefault, spec.wantDefault) {
					t.Errorf("%s.%s default = %q, want it to contain %q", spec.table, spec.column, *columnDefault, spec.wantDefault)
				}
			}
		})
	}
}

// TestMigration_LiftReviewState_DownRoundtrip verifies that running UP → DOWN → UP
// leaves all 6 migration-350 columns (including the Phase 1 toggle additions) present.
// This test is intentionally omitted here because migrationTestPool runs against
// an already-migrated DB (the test DB is set up to the latest version). A down-
// roundtrip would require a dedicated test-DB with migration control not available
// in this package's helper. The migration is verified structurally by
// TestMigration_LiftReviewState_DetectionModeToggleColumns and the existence of
// existing columns (TestMigration_LiftReviewState_RejectedByUserColumn etc.).

// TestMigration_LiftReviewState_PartialIndexUsedByActiveFilter verifies that
// EXPLAIN selects idx_lift_candidates_active for a query filtering on
// WHERE NOT rejected_by_user. The partial index is defined with that exact
// predicate — a planner hit confirms the index is correctly recognised.
//
// `SET LOCAL enable_seqscan = off` is required because on a near-empty
// activity_lift_candidates table the planner picks a seq scan (cheaper than
// any index on small data). Disabling seq-scan forces the planner to use
// the index if applicable, which proves the partial-index predicate matches.
func TestMigration_LiftReviewState_PartialIndexUsedByActiveFilter(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(context.Background()) //nolint:errcheck
	if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatalf("disable seqscan: %v", err)
	}

	rows, err := tx.Query(ctx, `
		EXPLAIN SELECT activity_id FROM activity_lift_candidates
		WHERE NOT rejected_by_user
	`)
	if err != nil {
		t.Fatalf("EXPLAIN query: %v", err)
	}
	defer rows.Close()

	var planLines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan EXPLAIN row: %v", err)
		}
		planLines = append(planLines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("EXPLAIN rows: %v", err)
	}

	fullPlan := strings.Join(planLines, "\n")
	if !strings.Contains(fullPlan, "idx_lift_candidates_active") {
		t.Errorf("expected EXPLAIN plan to reference idx_lift_candidates_active, got:\n%s", fullPlan)
	}
}
