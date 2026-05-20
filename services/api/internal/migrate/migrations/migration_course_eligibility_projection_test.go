package migrations_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMigration_CourseEligibilityProjection_CreatesTablesIndexesAndChecks(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for table, columns := range map[string][]string{
		"activity_trait_projection": {
			"activity_trait_bucket_key",
			"trait_policy_key",
			"trait_policy_version",
			"source_hash",
			"sport_type",
			"sub_type",
			"is_indoor",
			"is_virtual",
			"is_electric",
			"activity_form",
			"environment_state",
		},
		"target_course_eligibility_projection": {
			"target_kind",
			"target_id",
			"target_version_id",
			"matching_contract_key",
			"matching_contract_version",
			"activity_trait_bucket_key",
			"policy_key",
			"policy_version",
			"source_hash",
			"compatibility_preset",
			"activity_group",
			"match_allowed",
			"completion_allowed",
			"rank_allowed",
			"leaderboard_partition_template",
		},
		"segment_efforts": {
			"activity_trait_bucket_key",
			"eligibility_policy_key",
			"eligibility_policy_version",
			"eligibility_source_hash",
		},
	} {
		for _, column := range columns {
			var exists bool
			if err := pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.columns
					WHERE table_schema = 'public'
					  AND table_name = $1
					  AND column_name = $2
				)`, table, column).Scan(&exists); err != nil {
				t.Fatalf("query column %s.%s: %v", table, column, err)
			}
			if !exists {
				t.Fatalf("missing column %s.%s", table, column)
			}
		}
	}

	for _, index := range []string{
		"idx_tcep_candidate_lookup",
		"idx_tcep_target_lookup",
		"idx_tcep_filter_preset",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1)`, index).Scan(&exists); err != nil {
			t.Fatalf("query index %s: %v", index, err)
		}
		if !exists {
			t.Fatalf("missing index %s", index)
		}
	}
}

func TestMigration_CourseEligibilityProjection_RejectsInvalidContract(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, _, segmentID, versionID := insertCourseEligibilityFixture(t, ctx, pool)
	bucketKey := insertCourseEligibilityBucket(t, ctx, pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO target_course_eligibility_projection (
			target_kind, target_id, target_version_id, matching_contract_key, matching_contract_version,
			activity_trait_bucket_key, policy_key, policy_version, source_hash, compatibility_preset,
			activity_group, match_allowed, completion_allowed, rank_allowed, leaderboard_partition_template
		)
		VALUES ('segment', $1, $2, 'other', 1, $3, 'course_eligibility', 1, 'hash',
		        'standard', 'bike', true, true, true, '{}')
	`, segmentID, versionID, bucketKey)
	if err == nil {
		t.Fatal("invalid matching contract was accepted")
	}
}

func TestMigration_CourseEligibilityProjection_RejectsDenyOnlyRows(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, _, segmentID, versionID := insertCourseEligibilityFixture(t, ctx, pool)
	bucketKey := insertCourseEligibilityBucket(t, ctx, pool)

	_, err := pool.Exec(ctx, `
		INSERT INTO target_course_eligibility_projection (
			target_kind, target_id, target_version_id, matching_contract_key, matching_contract_version,
			activity_trait_bucket_key, policy_key, policy_version, source_hash, compatibility_preset,
			activity_group, match_allowed, completion_allowed, rank_allowed, leaderboard_partition_template
		)
		VALUES ('segment', $1, $2, 'course_gps_v1', 1, $3, 'course_eligibility', 1, 'hash',
		        'standard', 'bike', false, false, false, '{}')
	`, segmentID, versionID, bucketKey)
	if err == nil {
		t.Fatal("deny-only projection row was accepted")
	}
}

func TestMigration_CourseEligibilityProjection_CascadesWhenSegmentVersionIsDeleted(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, _, segmentID, versionID := insertCourseEligibilityFixture(t, ctx, pool)
	bucketKey := insertCourseEligibilityBucket(t, ctx, pool)

	if _, err := pool.Exec(ctx, `
		INSERT INTO target_course_eligibility_projection (
			target_kind, target_id, target_version_id, matching_contract_key, matching_contract_version,
			activity_trait_bucket_key, policy_key, policy_version, source_hash, compatibility_preset,
			activity_group, match_allowed, completion_allowed, rank_allowed, leaderboard_partition_template
		)
		VALUES ('segment', $1, $2, 'course_gps_v1', 1, $3, 'course_eligibility', 1, 'hash',
		        'standard', 'bike', true, true, true, '{}')
	`, segmentID, versionID, bucketKey); err != nil {
		t.Fatalf("insert projection row: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM segment_versions WHERE id = $1`, versionID); err != nil {
		t.Fatalf("delete segment version: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM target_course_eligibility_projection WHERE target_version_id = $1`, versionID).Scan(&count); err != nil {
		t.Fatalf("count projection rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("projection rows after segment version delete = %d, want 0", count)
	}
}

func TestMigration_CourseEligibilityProjection_BackfillsEffortBucketKey(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, activityID, segmentID, versionID := insertCourseEligibilityFixture(t, ctx, pool)
	effortID := insertCourseEligibilityEffort(t, ctx, pool, activityID, segmentID, versionID, "running", "")

	if _, err := pool.Exec(ctx, `UPDATE segment_efforts SET activity_trait_bucket_key = NULL WHERE id = $1`, effortID); err != nil {
		t.Fatalf("clear bucket: %v", err)
	}
	if _, err := pool.Exec(ctx, courseEligibilityBackfillSQL); err != nil {
		t.Fatalf("run backfill: %v", err)
	}
	var got string
	if err := pool.QueryRow(ctx, `SELECT activity_trait_bucket_key FROM segment_efforts WHERE id = $1`, effortID).Scan(&got); err != nil {
		t.Fatalf("query bucket: %v", err)
	}
	if got != "atb:v1:sport=running:sub=_:indoor=0:virtual=0:electric=0" {
		t.Fatalf("bucket = %q", got)
	}
}

func TestMigration_CourseEligibilityProjection_BackfillsEffortBucketKeyFromActivityModifiers(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, activityID, segmentID, versionID := insertCourseEligibilityFixture(t, ctx, pool)
	if _, err := pool.Exec(ctx, `UPDATE activities SET is_indoor = true, is_virtual = true, is_electric = true WHERE id = $1`, activityID); err != nil {
		t.Fatalf("set activity modifiers: %v", err)
	}
	effortID := insertCourseEligibilityEffort(t, ctx, pool, activityID, segmentID, versionID, "cycling", "mountain")

	if _, err := pool.Exec(ctx, `UPDATE segment_efforts SET activity_trait_bucket_key = NULL WHERE id = $1`, effortID); err != nil {
		t.Fatalf("clear bucket: %v", err)
	}
	if _, err := pool.Exec(ctx, courseEligibilityBackfillSQL); err != nil {
		t.Fatalf("run backfill: %v", err)
	}
	var got string
	if err := pool.QueryRow(ctx, `SELECT activity_trait_bucket_key FROM segment_efforts WHERE id = $1`, effortID).Scan(&got); err != nil {
		t.Fatalf("query bucket: %v", err)
	}
	if got != "atb:v1:sport=cycling:sub=mountain:indoor=1:virtual=1:electric=1" {
		t.Fatalf("bucket = %q", got)
	}
}

const courseEligibilityBackfillSQL = `
UPDATE segment_efforts se
SET activity_trait_bucket_key =
    'atb:v1:sport=' || lower(trim(COALESCE(NULLIF(se.sport_type, ''), 'generic'))) ||
    ':sub=' || lower(trim(COALESCE(NULLIF(NULLIF(se.sub_type, ''), 'generic'), '_'))) ||
    ':indoor=' || CASE WHEN COALESCE(a.is_indoor, false) THEN '1' ELSE '0' END ||
    ':virtual=' || CASE WHEN COALESCE(a.is_virtual, false) THEN '1' ELSE '0' END ||
    ':electric=' || CASE WHEN COALESCE(a.is_electric, false) THEN '1' ELSE '0' END,
    eligibility_policy_key = COALESCE(NULLIF(se.eligibility_policy_key, ''), 'course_eligibility'),
    eligibility_policy_version = COALESCE(se.eligibility_policy_version, 1)
FROM activities a
WHERE a.id = se.activity_id
  AND se.activity_trait_bucket_key IS NULL
`

func insertCourseEligibilityFixture(t *testing.T, ctx context.Context, pool courseEligibilityExecer) (userID, activityID, segmentID, versionID string) {
	t.Helper()
	userID = uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, userID, "courseelig_"+userID[:8], "courseelig_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	activityID = uuid.NewString()
	startedAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (
			id, user_id, file_hash, title, sport_type, sub_type,
			is_indoor, is_virtual, is_electric, started_at, visibility, enrichment_status
		)
		VALUES ($1, $2, $3, 'Course eligibility migration activity', 'cycling', 'mountain',
		        false, false, false, $4, 'public', 'complete')
	`, activityID, userID, "course-eligibility-"+activityID, startedAt); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	segmentID = uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, elevation_kind)
		VALUES ($1, $2, 'Course eligibility migration segment', 'bike', 'standard', 'hilly')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	versionID = uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, start_point, end_point,
			valid_from, valid_from_at, validity_basis, created_by
		)
		VALUES (
			$1, $2, 1,
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			ST_GeomFromText('POINT(8 49)', 4326),
			ST_GeomFromText('POINT(8.01 49.01)', 4326),
			'2026-01-01',
			'2026-01-01T00:00:00Z',
			'editor_asserted',
			$3
		)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	return userID, activityID, segmentID, versionID
}

func insertCourseEligibilityBucket(t *testing.T, ctx context.Context, pool courseEligibilityExecer) string {
	t.Helper()
	key := "atb:v1:sport=cycling:sub=mountain:indoor=0:virtual=0:electric=0"
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_trait_projection (
			activity_trait_bucket_key, trait_policy_key, trait_policy_version, source_hash,
			sport_type, sub_type, is_indoor, is_virtual, is_electric,
			movement_families, propulsion_modes, equipment_families, activity_form, environment_state
		)
		VALUES ($1, 'activity_traits', 1, 'hash', 'cycling', 'mountain', false, false, false,
		        ARRAY['pedal'], ARRAY['human'], ARRAY['bike'], 'single', 'real_world')
		ON CONFLICT (activity_trait_bucket_key) DO NOTHING
	`, key); err != nil {
		t.Fatalf("insert activity trait bucket: %v", err)
	}
	return key
}

func insertCourseEligibilityEffort(t *testing.T, ctx context.Context, pool courseEligibilityExecer, activityID, segmentID, versionID, sportType, subType string) string {
	t.Helper()
	effortID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			id, segment_id, segment_version_id, activity_id, user_id, started_at,
			elapsed_time_s, sport_type, sub_type
		)
		SELECT $1, $2, $3, $4, a.user_id, a.started_at, 300, $5, $6
		FROM activities a
		WHERE a.id = $4
	`, effortID, segmentID, versionID, activityID, sportType, subType); err != nil {
		t.Fatalf("insert effort: %v", err)
	}
	return effortID
}

type courseEligibilityExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}
