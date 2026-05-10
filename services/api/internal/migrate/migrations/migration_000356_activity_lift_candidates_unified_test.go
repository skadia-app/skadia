package migrations_test

// Integration tests for migration 000356_activity_lift_candidates_unified.
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
	"time"

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

// TestMigration_LiftCandidatesUnified_RejectsUppercaseSource verifies that the
// CHECK constraint is case-sensitive: 'OSM' (uppercase) must be rejected even
// though 'osm' (lowercase) is valid. This documents the intended contract and
// guards against accidental case-insensitive constraint rewrites.
func TestMigration_LiftCandidatesUnified_RejectsUppercaseSource(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"liftosmupper_"+userID[:8],
		"liftosmupper_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	activityID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, started_at)
		VALUES ($1, $2, 'mig349_upper', 'skiing', NOW())
	`, activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM activities WHERE id = $1", activityID)
	})

	_, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, source)
		VALUES ($1, NOW(), NOW() + interval '60s', 60, 100, 'OSM')
	`, activityID)
	if err == nil {
		t.Fatal("expected CHECK constraint violation for source='OSM' (uppercase), got success")
	}
	if !strings.Contains(err.Error(), "check") && !strings.Contains(err.Error(), "constraint") {
		t.Errorf("expected CHECK constraint error for 'OSM', got: %v", err)
	}
}

// TestMigration_LiftCandidatesUnified_SegmentEffortFlagsDefaultFalse verifies
// that the three audit boolean columns added to segment_efforts have DEFAULT
// false on rows that pre-date the migration. The test inserts a segment_effort
// row without specifying the flags and reads them back to confirm the default.
//
// Note: segment_efforts is a TimescaleDB hypertable with composite PK
// (id, started_at) — we build the minimal chain: user → activity → segment →
// segment_version → segment_effort.
func TestMigration_LiftCandidatesUnified_SegmentEffortFlagsDefaultFalse(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"liftseflag_"+userID[:8],
		"liftseflag_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	activityID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, title, sport_type, started_at)
		VALUES ($1, $2, 'mig349_seflag', 'skiing', NOW())
	`, activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM activities WHERE id = $1", activityID)
	})

	segID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments
			(id, name, segment_type, distance_m, elevation_gain_m, sport_group, created_by)
		VALUES ($1, 'mig349_seg', 'lift', 200, 80, 'skiing', $2)
	`, segID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM segments WHERE id = $1", segID)
	})

	versionID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions
			(id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1,
		        ST_GeomFromText('LINESTRING(11.0 47.0, 11.005 47.0)', 4326),
		        CURRENT_DATE, $3)
	`, versionID, segID, userID); err != nil {
		t.Fatalf("insert segment_version: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM segment_versions WHERE id = $1", versionID)
	})

	// Insert a segment_effort without specifying the three audit flag columns.
	effortID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, segment_id, segment_version_id, activity_id, user_id,
			 started_at, elapsed_time_s, sport_type, sub_type, in_leaderboard)
		VALUES ($1, $2, $3, $4, $5, $6, 60, 'skiing', 'generic', false)
	`, effortID, segID, versionID, activityID, userID, now); err != nil {
		t.Fatalf("insert segment_effort: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM segment_efforts WHERE id = $1", effortID)
	})

	var isLiftMatch, isMethodAOnly, isCableHoldMerge bool
	if err := pool.QueryRow(ctx, `
		SELECT is_lift_segment_match, is_method_a_only, is_cable_hold_merge
		FROM segment_efforts WHERE id = $1
	`, effortID).Scan(&isLiftMatch, &isMethodAOnly, &isCableHoldMerge); err != nil {
		t.Fatalf("query segment_effort flags: %v", err)
	}
	if isLiftMatch {
		t.Error("is_lift_segment_match = true, want false (DEFAULT should apply)")
	}
	if isMethodAOnly {
		t.Error("is_method_a_only = true, want false (DEFAULT should apply)")
	}
	if isCableHoldMerge {
		t.Error("is_cable_hold_merge = true, want false (DEFAULT should apply)")
	}
}

// TestMigration_LiftCandidatesUnified_IndexUsedByDedupQuery verifies that
// EXPLAIN selects idx_alc_heuristic_only_confirmed for the recomputeLiftStats
// query pattern (WHERE confirmed=true AND segment_effort_id IS NULL).
// An index scan here confirms the partial index predicate matches the query.
//
// `SET LOCAL enable_seqscan = off` is required because on a near-empty
// activity_lift_candidates table the planner picks a seq scan (cheaper than
// any index on small data). Disabling seq-scan forces the planner to use
// the index if applicable, which proves the partial-index predicate matches.
func TestMigration_LiftCandidatesUnified_IndexUsedByDedupQuery(t *testing.T) {
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
		WHERE confirmed = true AND segment_effort_id IS NULL
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
	if !strings.Contains(fullPlan, "idx_alc_heuristic_only_confirmed") {
		t.Errorf("expected EXPLAIN plan to reference idx_alc_heuristic_only_confirmed, got:\n%s", fullPlan)
	}
}
