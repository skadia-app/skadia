package migrations_test

// Integration tests for migration 000349_activity_lift_candidates_unified.
//
// Phase 4c — OSM-Snap-Pipeline. The migration:
//   1. Adds activity_lift_candidates.source TEXT NOT NULL DEFAULT 'heuristic'
//      with CHECK (source IN ('heuristic','osm')).
//   2. Adds activity_lift_candidates.segment_effort_id UUID (nullable)
//      — Phase-4c snap stages set this to the inserted segment_effort id so
//      lift-stats recompute can dedup heuristic+OSM in a single query.
//   3. Adds segment_efforts.is_lift_segment_match, .is_method_a_only,
//      .is_cable_hold_merge BOOLEAN audit flags (Stage 1/2/3 markers).
//
// Note on segment_effort_id type: segment_efforts has a composite PK
// (id UUID, started_at TIMESTAMPTZ) due to TimescaleDB hypertable. We
// therefore use a UUID link on segment_effort_id. NO foreign key constraint is
// added because composite PK targets cannot be referenced by a single-column FK
// in Postgres; the dedup contract is maintained by the writing code (snap.go)
// and audited by nullability semantics.
//
// Tests are skipped when TEST_DATABASE_URL is unset.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestMigration_LiftCandidatesUnified_HasSourceColumn verifies the source
// column exists with the expected default and CHECK constraint.
func TestMigration_LiftCandidatesUnified_HasSourceColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var dataType, columnDefault string
	var isNullable string
	err := pool.QueryRow(ctx, `
		SELECT data_type, column_default, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'activity_lift_candidates' AND column_name = 'source'
	`).Scan(&dataType, &columnDefault, &isNullable)
	if err != nil {
		t.Fatalf("query column metadata: %v", err)
	}
	if dataType != "text" {
		t.Errorf("source data_type = %q, want %q", dataType, "text")
	}
	if isNullable != "NO" {
		t.Errorf("source is_nullable = %q, want %q", isNullable, "NO")
	}
	if !strings.Contains(columnDefault, "heuristic") {
		t.Errorf("source default = %q, want it to contain 'heuristic'", columnDefault)
	}
}

// TestMigration_LiftCandidatesUnified_RejectsInvalidSource verifies the CHECK
// constraint blocks anything outside {'heuristic','osm'}.
func TestMigration_LiftCandidatesUnified_RejectsInvalidSource(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"liftunif_"+userID[:8],
		"liftunif_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	activityID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, started_at)
		VALUES ($1, $2, 'mig999_test', 'skiing', NOW())
	`, activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM activities WHERE id = $1", activityID)
	})

	_, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, source)
		VALUES ($1, NOW(), NOW() + interval '60s', 60, 100, 'manual')
	`, activityID)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for source='manual', got success")
	}
	if !strings.Contains(err.Error(), "check") && !strings.Contains(err.Error(), "constraint") {
		t.Errorf("expected CHECK error, got: %v", err)
	}
}

// TestMigration_LiftCandidatesUnified_AcceptsBothSources verifies that valid
// values 'heuristic' and 'osm' both pass.
func TestMigration_LiftCandidatesUnified_AcceptsBothSources(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"liftunifok_"+userID[:8],
		"liftunifok_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	activityID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, started_at)
		VALUES ($1, $2, 'mig999_test', 'skiing', NOW())
	`, activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM activities WHERE id = $1", activityID)
	})

	for i, src := range []string{"heuristic", "osm"} {
		offsetS := i * 100
		_, err := pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, source)
			VALUES ($1,
			        NOW() + make_interval(secs => $2::int),
			        NOW() + make_interval(secs => $2::int + 60),
			        60, 100, $3)
		`, activityID, offsetS, src)
		if err != nil {
			t.Errorf("insert source=%q failed: %v", src, err)
		}
	}
}

// TestMigration_LiftCandidatesUnified_HasSegmentEffortIDColumn verifies the
// segment_effort_id column is a nullable UUID (no FK because segment_efforts
// has a composite PK that cannot be referenced).
func TestMigration_LiftCandidatesUnified_HasSegmentEffortIDColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var dataType, isNullable string
	err := pool.QueryRow(ctx, `
		SELECT data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'activity_lift_candidates' AND column_name = 'segment_effort_id'
	`).Scan(&dataType, &isNullable)
	if err != nil {
		t.Fatalf("query column metadata: %v", err)
	}
	if dataType != "uuid" {
		t.Errorf("segment_effort_id data_type = %q, want %q", dataType, "uuid")
	}
	if isNullable != "YES" {
		t.Errorf("segment_effort_id is_nullable = %q, want %q", isNullable, "YES")
	}
}

// TestMigration_LiftCandidatesUnified_HasSegmentEffortAuditFlags verifies the
// three new boolean audit columns on segment_efforts.
func TestMigration_LiftCandidatesUnified_HasSegmentEffortAuditFlags(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for _, col := range []string{"is_lift_segment_match", "is_method_a_only", "is_cable_hold_merge"} {
		var dataType, isNullable, columnDefault string
		err := pool.QueryRow(ctx, `
			SELECT data_type, is_nullable, COALESCE(column_default, '')
			FROM information_schema.columns
			WHERE table_name = 'segment_efforts' AND column_name = $1
		`, col).Scan(&dataType, &isNullable, &columnDefault)
		if err != nil {
			t.Errorf("query segment_efforts.%s: %v", col, err)
			continue
		}
		if dataType != "boolean" {
			t.Errorf("segment_efforts.%s data_type = %q, want %q", col, dataType, "boolean")
		}
		if !strings.Contains(columnDefault, "false") {
			t.Errorf("segment_efforts.%s default = %q, want default false", col, columnDefault)
		}
	}
}

// TestMigration_LiftCandidatesUnified_PartialIndexExists verifies the partial
// index for the heuristic-only dedup query (segment_effort_id IS NULL) exists.
// The recomputeLiftStats query uses WHERE confirmed=true AND segment_effort_id IS NULL
// — a partial index keeps it fast even on large activity_lift_candidates tables.
func TestMigration_LiftCandidatesUnified_PartialIndexExists(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'activity_lift_candidates'
			  AND indexname = 'idx_alc_heuristic_only_confirmed'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("query pg_indexes: %v", err)
	}
	if !exists {
		t.Error("expected partial index idx_alc_heuristic_only_confirmed to exist")
	}
}
