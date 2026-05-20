package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigration_SegmentVersioningLeaderboardIntegrity_AddsRevisionRecalcAndEquivalenceTables(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for table, columns := range map[string][]string{
		"segment_leaderboard_equivalence_groups": {
			"id", "segment_id", "change_key", "policy_key", "policy_version",
			"equivalence_kind", "created_by", "metadata",
		},
		"segment_leaderboard_equivalence_group_versions": {
			"group_id", "segment_version_id", "leaderboard_surface", "joined_at", "retired_at",
		},
		"segment_recalculation_segment_state": {"segment_id", "generation", "updated_at"},
		"segment_recalculation_runs": {
			"id", "segment_id", "actor_user_id", "operation_key_hash", "idempotency_key",
			"generation", "status", "claimed_at", "claimed_by",
		},
		"segment_recalculation_run_activities": {
			"run_id", "activity_id", "user_id", "status", "error_code", "error_message",
			"attempt_count", "claim_token", "claimed_at", "claimed_until", "claimed_by",
		},
		"segment_version_revisions": {
			"id", "segment_id", "actor_user_id", "operation", "snapshot_schema_key",
			"snapshot_schema_version", "before_snapshot", "after_snapshot",
			"before_snapshot_sha256", "after_snapshot_sha256", "recalculation_run_id",
			"rollback_of_revision_id",
		},
		"segment_efforts": {
			"superseded_at", "superseded_reason", "superseded_by_recalculation_run_id",
			"created_by_recalculation_run_id", "leaderboard_equivalence_group_id",
		},
	} {
		for _, column := range columns {
			assertColumnExists(t, ctx, pool, table, column)
		}
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_BackfillsExplicitEquivalenceGroups(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, _, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)

	var groupID string
	if err := pool.QueryRow(ctx, `
		SELECT g.id
		FROM segment_leaderboard_equivalence_groups g
		JOIN segment_leaderboard_equivalence_group_versions gv ON gv.group_id = g.id
		WHERE g.segment_id = $1
		  AND gv.segment_version_id = $2
		  AND g.equivalence_kind = 'version_specific'
		  AND gv.leaderboard_surface = 'default'
		  AND gv.retired_at IS NULL
	`, segmentID, versionID).Scan(&groupID); err != nil {
		t.Fatalf("query equivalence group: %v", err)
	}
	if groupID == "" {
		t.Fatal("empty group id")
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_ActiveEffortPredicateUsesSupersededAt(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	assertIndexPredicateContains(t, ctx, pool, "idx_segment_efforts_active_match_identity", "superseded_at IS NULL")
	assertIndexPredicateContains(t, ctx, pool, "idx_segment_efforts_leaderboard_partition", "superseded_at IS NULL")
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_AllowsRemovedNoReplacementEffort(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)
	startedAt := time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)

	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, sport_type, sub_type,
			match_identity_key, leaderboard_equivalence_group_id
		)
		SELECT $1, $2, $3, $4, $5, 60, 'cycling', 'mountain',
		       $6,
		       gv.group_id
		FROM segment_leaderboard_equivalence_group_versions gv
		WHERE gv.segment_version_id = $2
		  AND gv.retired_at IS NULL
		LIMIT 1
	`, segmentID, versionID, activityID, userID, startedAt, "mig-plan2-removed:"+activityID); err != nil {
		t.Fatalf("insert effort: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE segment_efforts
		SET superseded_at = NOW(),
		    superseded_reason = 'recalculated_removed',
		    superseded_by_effort_id = NULL
		WHERE activity_id = $1
	`, activityID); err != nil {
		t.Fatalf("mark removed effort: %v", err)
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_DownRefusesUnsafeRemovedEfforts(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)
	startedAt := time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, sport_type, sub_type,
			match_identity_key, leaderboard_equivalence_group_id,
			superseded_at, superseded_reason
		)
		SELECT $1, $2, $3, $4, $5, 60, 'cycling', 'mountain',
		       $6,
		       gv.group_id,
		       NOW(), 'recalculated_removed'
		FROM segment_leaderboard_equivalence_group_versions gv
		WHERE gv.segment_version_id = $2
		  AND gv.retired_at IS NULL
		LIMIT 1
	`, segmentID, versionID, activityID, userID, startedAt, "mig-plan2-down:"+activityID); err != nil {
		t.Fatalf("insert removed effort: %v", err)
	}

	downSQL := readMigrationSQL(t, "*_segment_versioning_leaderboard_integrity.down.sql")
	_, err = tx.Exec(ctx, downSQL)
	if err == nil {
		t.Fatal("expected unsafe down migration to fail")
	}
	if !strings.Contains(err.Error(), "unsafe rollback") {
		t.Fatalf("down migration error = %v, want unsafe rollback", err)
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_RejectsInvalidStatusesAndPolicies(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, _, segmentID, _ := insertSegmentValidityFixture(t, ctx, pool)

	assertCheckRejects(t, ctx, pool, `
		INSERT INTO segment_leaderboard_equivalence_groups (
			segment_id, change_key, policy_key, policy_version, equivalence_kind
		)
		VALUES ($1, 'bad', 'segment_leaderboard_equivalence', 1, 'merged_magic')
	`, segmentID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin invalid status tx: %v", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO segment_recalculation_runs (
			segment_id, actor_user_id, operation_key_hash, idempotency_key,
			generation, reason, status
		)
		VALUES ($1, $2, 'op', 'idem', 1, 'test', 'paused')
	`, segmentID, userID)
	if err == nil {
		t.Fatal("expected invalid recalculation status to fail")
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_AddsKubernetesSafeClaimingFields(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for _, index := range []string{
		"idx_segment_recalc_runs_segment_status",
		"idx_segment_recalc_runs_status_claimed",
		"idx_segment_recalc_run_activities_status_claim",
		"idx_segment_recalc_run_activities_global_status",
	} {
		assertIndexExists(t, ctx, pool, index)
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_GDPRRegistryCoversUserLinkedTables(t *testing.T) {
	for _, table := range []string{
		"segment_leaderboard_equivalence_groups",
		"segment_leaderboard_equivalence_group_versions",
		"segment_recalculation_segment_state",
		"segment_recalculation_runs",
		"segment_recalculation_run_activities",
		"segment_version_revisions",
	} {
		if _, ok := auth.GDPRRegistry[table]; !ok {
			t.Fatalf("GDPR registry missing %s", table)
		}
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_SegmentDeleteCascadesNewTables(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)

	var groupID string
	if err := pool.QueryRow(ctx, `
		SELECT group_id
		FROM segment_leaderboard_equivalence_group_versions
		WHERE segment_version_id = $1
		  AND retired_at IS NULL
		LIMIT 1
	`, versionID).Scan(&groupID); err != nil {
		t.Fatalf("query group id: %v", err)
	}

	runID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_recalculation_segment_state (segment_id, generation)
		VALUES ($1, 1)
		ON CONFLICT (segment_id) DO UPDATE SET generation = EXCLUDED.generation
	`, segmentID); err != nil {
		t.Fatalf("insert recalculation state: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_recalculation_runs (
			id, segment_id, actor_user_id, operation_key_hash, idempotency_key,
			generation, reason
		)
		VALUES ($1, $2, $3, 'op-delete', 'idem-delete', 1, 'delete cascade test')
	`, runID, segmentID, userID); err != nil {
		t.Fatalf("insert recalculation run: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_recalculation_run_activities (run_id, activity_id, user_id)
		VALUES ($1, $2, $3)
	`, runID, activityID, userID); err != nil {
		t.Fatalf("insert recalculation run activity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_version_revisions (
			segment_id, actor_user_id, operation, before_snapshot, after_snapshot,
			before_snapshot_sha256, after_snapshot_sha256, recalculation_run_id
		)
		VALUES ($1, $2, 'validity_update', '{}'::jsonb, '{}'::jsonb, 'before', 'after', $3)
	`, segmentID, userID, runID); err != nil {
		t.Fatalf("insert segment revision: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id, started_at,
			elapsed_time_s, sport_type, sub_type, match_identity_key,
			leaderboard_equivalence_group_id
		)
			VALUES (
				$1, $2, $3, $4, '2026-05-14T09:30:00Z', 60, 'cycling', 'mountain',
				$5, $6
			)
	`, segmentID, versionID, activityID, userID, "delete-cascade-test:"+activityID, groupID); err != nil {
		t.Fatalf("insert cascade effort: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM segments WHERE id = $1`, segmentID); err != nil {
		t.Fatalf("delete segment: %v", err)
	}
	for _, table := range []string{
		"segment_recalculation_segment_state",
		"segment_recalculation_runs",
		"segment_version_revisions",
		"segment_leaderboard_equivalence_groups",
	} {
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE segment_id = $1`, segmentID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows after segment delete = %d, want 0", table, count)
		}
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_FillsActiveEffortEquivalenceGroup(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)

	var effortGroupID string
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id, started_at,
			elapsed_time_s, sport_type, sub_type, match_identity_key
		)
		VALUES ($1, $2, $3, $4, '2026-05-14T09:30:00Z', 60, 'cycling', 'mountain', $5)
	`, segmentID, versionID, activityID, userID, "mig-plan2-fill:"+activityID); err != nil {
		t.Fatalf("insert active effort without group: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT leaderboard_equivalence_group_id
		FROM segment_efforts
		WHERE activity_id = $1
	`, activityID).Scan(&effortGroupID); err != nil {
		t.Fatalf("query filled effort group: %v", err)
	}
	if effortGroupID == "" {
		t.Fatal("active effort group id was not filled")
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_RejectsCrossSegmentMembership(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, _, _, versionAID := insertSegmentValidityFixture(t, ctx, pool)
	_, _, segmentBID, _ := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)

	var groupBID string
	if err := pool.QueryRow(ctx, `
		SELECT id
		FROM segment_leaderboard_equivalence_groups
		WHERE segment_id = $1
		LIMIT 1
	`, segmentBID).Scan(&groupBID); err != nil {
		t.Fatalf("query second segment group: %v", err)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO segment_leaderboard_equivalence_group_versions (
			group_id, segment_version_id, leaderboard_surface, join_reason
		)
		VALUES ($1, $2, 'default', 'bad_cross_segment_test')
	`, groupBID, versionAID)
	if err == nil {
		t.Fatal("expected cross-segment membership to fail")
	}
	if !strings.Contains(err.Error(), "different segments") {
		t.Fatalf("cross-segment error = %v, want different segments", err)
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_RejectsMultiVersionVersionSpecificGroup(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, _, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)

	var groupID string
	if err := pool.QueryRow(ctx, `
		SELECT group_id
		FROM segment_leaderboard_equivalence_group_versions
		WHERE segment_version_id = $1
		  AND retired_at IS NULL
	`, versionID).Scan(&groupID); err != nil {
		t.Fatalf("query version_specific group: %v", err)
	}
	var secondVersionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO segment_versions (
			segment_id, version_number, track, start_point, end_point, valid_from, created_by,
			valid_from_at, validity_basis, matching_contract_key, matching_contract_version, published_at
		)
		SELECT $1, 2,
		       ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		       ST_StartPoint(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
		       ST_EndPoint(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
		       CURRENT_DATE, $2, '2027-01-01T00:00:00Z', 'editor_asserted',
		       'course_gps_v1', 1, NOW()
		RETURNING id
	`, segmentID, userID).Scan(&secondVersionID); err != nil {
		t.Fatalf("insert second version: %v", err)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO segment_leaderboard_equivalence_group_versions (
			group_id, segment_version_id, leaderboard_surface, join_reason
		)
		VALUES ($1, $2, 'default', 'bad_version_specific_test')
	`, groupID, secondVersionID)
	if err == nil {
		t.Fatal("expected multi-version version_specific group to fail")
	}
	if !strings.Contains(err.Error(), "version_specific") {
		t.Fatalf("version_specific cardinality error = %v", err)
	}
}

func TestMigration_SegmentVersioningLeaderboardIntegrity_RejectsEffortInactiveMembership(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	applySegmentVersioningLeaderboardIntegrityMigration(t, ctx, pool)

	var groupID string
	if err := pool.QueryRow(ctx, `
		SELECT group_id
		FROM segment_leaderboard_equivalence_group_versions
		WHERE segment_version_id = $1
		  AND retired_at IS NULL
	`, versionID).Scan(&groupID); err != nil {
		t.Fatalf("query group: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE segment_leaderboard_equivalence_group_versions
		SET retired_at = NOW()
		WHERE group_id = $1 AND segment_version_id = $2
	`, groupID, versionID); err != nil {
		t.Fatalf("retire membership: %v", err)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id, started_at,
			elapsed_time_s, sport_type, sub_type, match_identity_key,
			leaderboard_equivalence_group_id
		)
		VALUES ($1, $2, $3, $4, '2026-05-14T09:30:00Z', 60, 'cycling', 'mountain', $5, $6)
	`, segmentID, versionID, activityID, userID, "mig-plan2-inactive:"+activityID, groupID)
	if err == nil {
		t.Fatal("expected inactive effort membership to fail")
	}
	if !strings.Contains(err.Error(), "not active") {
		t.Fatalf("inactive membership error = %v, want not active", err)
	}
}

func readMigrationSQL(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob migration sql: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one migration for %s, got %d: %v", pattern, len(matches), matches)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	return string(body)
}

func applySegmentVersioningLeaderboardIntegrityMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		WITH missing_versions AS (
			SELECT sv.id AS version_id,
			       sv.segment_id,
			       sv.version_number
			FROM segment_versions sv
			WHERE sv.matching_contract_key = 'course_gps_v1'
			  AND sv.matching_contract_version = 1
			  AND NOT EXISTS (
			      SELECT 1
			      FROM segment_leaderboard_equivalence_group_versions gv
			      WHERE gv.segment_version_id = sv.id
			        AND gv.leaderboard_surface = 'default'
			        AND gv.retired_at IS NULL
			  )
		),
		inserted AS (
			INSERT INTO segment_leaderboard_equivalence_groups (
				segment_id, change_key, policy_key, policy_version, equivalence_kind, reason, metadata
			)
			SELECT mv.segment_id,
			       'legacy_version_specific:' || mv.version_id::text,
			       'segment_leaderboard_equivalence',
			       1,
			       'version_specific',
			       'legacy_version_specific_backfill',
			       jsonb_build_object(
			           'legacy_policy', 'legacy_segment_wide',
			           'backfilled_version_id', mv.version_id::text,
			           'version_number', mv.version_number,
			           'merge_policy', 'no_automatic_historical_merge'
			       )
			FROM missing_versions mv
			ON CONFLICT (segment_id, policy_key, policy_version, change_key) DO UPDATE
				SET metadata = EXCLUDED.metadata
			RETURNING id, metadata
		)
		INSERT INTO segment_leaderboard_equivalence_group_versions (
			group_id, segment_version_id, leaderboard_surface, join_reason
		)
		SELECT inserted.id,
		       (inserted.metadata->>'backfilled_version_id')::uuid,
		       'default',
		       'legacy_version_specific_backfill'
		FROM inserted
		ON CONFLICT (group_id, segment_version_id, leaderboard_surface) DO NOTHING
	`); err != nil {
		t.Fatalf("backfill missing segment version equivalence groups: %v", err)
	}
}

func assertColumnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists); err != nil {
		t.Fatalf("query column %s.%s: %v", table, column, err)
	}
	if !exists {
		t.Fatalf("missing column %s.%s", table, column)
	}
}

func assertIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE indexname = $1
		)`, index).Scan(&exists); err != nil {
		t.Fatalf("query index %s: %v", index, err)
	}
	if !exists {
		t.Fatalf("missing index %s", index)
	}
}

func assertIndexPredicateContains(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index, want string) {
	t.Helper()
	var predicate string
	if err := pool.QueryRow(ctx, `
		SELECT pg_get_expr(i.indpred, i.indrelid)
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = $1
	`, index).Scan(&predicate); err != nil {
		t.Fatalf("query index predicate %s: %v", index, err)
	}
	if !strings.Contains(predicate, want) {
		t.Fatalf("index %s predicate = %q, want containing %q", index, predicate, want)
	}
}
