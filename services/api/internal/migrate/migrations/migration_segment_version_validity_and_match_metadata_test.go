package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func readSegmentValidityMigrationSQL(t *testing.T) string {
	t.Helper()
	matches, err := filepath.Glob("*_segment_version_validity_and_match_metadata.up.sql")
	if err != nil {
		t.Fatalf("glob migration sql: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one segment validity migration, got %d: %v", len(matches), matches)
	}
	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read %s: %v", matches[0], err)
	}
	return string(body)
}

func TestMigration_SegmentVersionValidityAndMatchMetadata_ColumnsIndexesAndChecks(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for table, columns := range map[string][]string{
		"segment_versions": {
			"matching_contract_key",
			"matching_contract_version",
			"valid_from_at",
			"valid_until_at",
			"validity_basis",
			"published_at",
		},
		"segment_efforts": {
			"matching_contract_key",
			"matching_contract_version",
			"match_source_type",
			"match_source_id",
			"match_identity_key",
			"occurrence_index",
			"occurrence_hash",
			"superseded_by_effort_id",
			"leaderboard_sport_type",
			"leaderboard_sub_type",
			"leaderboard_equipment_class",
			"leaderboard_assist_class",
			"leaderboard_indoor_policy_state",
			"leaderboard_virtual_policy_state",
			"leaderboard_electric_policy_state",
			"leaderboard_direction",
			"leaderboard_version_policy_key",
			"leaderboard_contract_key",
			"leaderboard_contract_version",
		},
		"segments": {"elevation_kind"},
	} {
		for _, column := range columns {
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
	}

	for _, index := range []string{
		"idx_segment_versions_course_gps_v1_validity",
		"idx_segment_efforts_active_match_identity",
		"idx_segment_efforts_leaderboard_partition",
		"idx_segments_elevation_kind",
	} {
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

	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	startedAt := time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, sport_type, sub_type
		)
		VALUES ($1, $2, $3, $4, $5, 60, 'cycling', 'mountain')
	`, segmentID, versionID, activityID, userID, startedAt); err != nil {
		t.Fatalf("insert effort for checks: %v", err)
	}

	assertCheckRejects(t, ctx, pool, `UPDATE segment_versions SET matching_contract_key = 'other' WHERE id = $1`, versionID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_versions SET matching_contract_version = 2 WHERE id = $1`, versionID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_versions SET validity_basis = 'guess' WHERE id = $1`, versionID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET matching_contract_key = 'other' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET matching_contract_version = 2 WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET match_source_type = 'manual' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_contract_key = 'other' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_contract_version = 2 WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_version_policy_key = 'version_specific' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_direction = 'sideways' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_assist_class = 'motor' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_indoor_policy_state = 'mixed' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_virtual_policy_state = 'mixed' WHERE activity_id = $1`, activityID)
	assertCheckRejects(t, ctx, pool, `UPDATE segment_efforts SET leaderboard_electric_policy_state = 'mixed' WHERE activity_id = $1`, activityID)
}

func TestMigration_SegmentVersionValidityAndMatchMetadata_RejectsOverlappingVersionValidity(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, _, segmentID, _ := insertSegmentValidityFixture(t, ctx, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `ALTER TABLE segment_versions DISABLE TRIGGER trg_segment_versions_validity_defaults`); err != nil {
		t.Fatalf("disable validity trigger: %v", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO segment_versions (
			segment_id, version_number, track, valid_from, created_by,
			valid_from_at, valid_until_at, validity_basis
		)
		VALUES (
			$1, 2, ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326),
			'2026-01-15', $2, '2026-01-15T00:00:00Z', '2026-03-01T00:00:00Z', 'editor_asserted'
		)`, segmentID, userID)
	if err == nil {
		t.Fatal("expected overlapping version validity to fail")
	}
}

func TestMigration_SegmentVersionValidityAndMatchMetadata_LegacyDirectEffortInsertGetsDefaults(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	startedAt := time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)

	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			segment_id, segment_version_id, activity_id, user_id,
			started_at, elapsed_time_s, is_reverse, sport_type, sub_type
		)
		VALUES ($1, $2, $3, $4, $5, 60, false, 'cycling', 'mountain')
	`, segmentID, versionID, activityID, userID, startedAt); err != nil {
		t.Fatalf("legacy effort insert: %v", err)
	}

	wantIdentity := "platform_activity:" + activityID +
		":segment:" + segmentID +
		":version:" + versionID +
		":contract:course_gps_v1:1:direction:forward:start:2026-05-14T09:30:00.000000Z:occurrence:0"

	var matchSourceID, matchIdentity, lbSport, lbSub, assist, indoor, virtual, electric, direction string
	var contractVersion int
	if err := pool.QueryRow(ctx, `
		SELECT match_source_id, match_identity_key, matching_contract_version,
		       leaderboard_sport_type, leaderboard_sub_type,
		       leaderboard_assist_class, leaderboard_indoor_policy_state,
		       leaderboard_virtual_policy_state, leaderboard_electric_policy_state,
		       leaderboard_direction
		FROM segment_efforts
		WHERE activity_id = $1`, activityID).Scan(
		&matchSourceID, &matchIdentity, &contractVersion,
		&lbSport, &lbSub, &assist, &indoor, &virtual, &electric, &direction,
	); err != nil {
		t.Fatalf("query defaults: %v", err)
	}
	if matchSourceID != activityID {
		t.Fatalf("match_source_id = %q, want %q", matchSourceID, activityID)
	}
	if matchIdentity != wantIdentity {
		t.Fatalf("match_identity_key = %q, want %q", matchIdentity, wantIdentity)
	}
	if contractVersion != 1 || lbSport != "cycling" || lbSub != "mountain" ||
		assist != "electric" || indoor != "indoor" || virtual != "virtual" || electric != "electric" || direction != "forward" {
		t.Fatalf("unexpected defaults: version=%d sport=%s/%s assist=%s indoor=%s virtual=%s electric=%s direction=%s",
			contractVersion, lbSport, lbSub, assist, indoor, virtual, electric, direction)
	}
}

func TestMigration_SegmentVersionValidityAndMatchMetadata_LiftDirectInsertShapesGetDefaults(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID, activityID, segmentID, versionID := insertSegmentValidityFixture(t, ctx, pool)
	startedAt := time.Date(2026, 5, 14, 9, 45, 0, 0, time.UTC)
	effortID := uuid.NewString()

	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard,
			 is_lift_segment_match, is_method_a_only, is_cable_hold_merge)
		VALUES ($1, $2, $3, $4, $5, $6, 120, 500, 100, false, 'cycling', 'mountain', false, true, true, false)
	`, effortID, activityID, segmentID, versionID, userID, startedAt); err != nil {
		t.Fatalf("lift single insert: %v", err)
	}

	batchID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard,
			 is_lift_segment_match, is_method_a_only, is_cable_hold_merge)
		SELECT
			u.id, u.activity_id, u.segment_id, u.segment_version_id, a.user_id,
			u.started_at, u.elapsed_time_s, u.distance_m, u.elevation_gain_m,
			u.is_reverse, a.sport_type, COALESCE(NULLIF(a.sub_type, ''), 'generic'), false,
			true, u.is_method_a_only, false
		FROM UNNEST(
			$1::uuid[], $2::uuid[], $3::uuid[], $4::uuid[],
			$5::timestamptz[], $6::int[], $7::float8[], $8::float8[],
			$9::bool[], $10::bool[]
		) AS u(id, activity_id, segment_id, segment_version_id,
		       started_at, elapsed_time_s, distance_m, elevation_gain_m,
		       is_reverse, is_method_a_only)
		JOIN activities a ON a.id = u.activity_id AND a.user_id = $11
	`, []string{batchID}, []string{activityID}, []string{segmentID}, []string{versionID},
		[]time.Time{startedAt.Add(5 * time.Minute)}, []int{90}, []float64{300}, []float64{50},
		[]bool{true}, []bool{true}, userID); err != nil {
		t.Fatalf("lift batch insert: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT id, match_identity_key, in_leaderboard,
		       is_lift_segment_match, is_method_a_only, is_cable_hold_merge
		FROM segment_efforts
		WHERE id = ANY($1::uuid[])
		ORDER BY id`, []string{effortID, batchID})
	if err != nil {
		t.Fatalf("query lift efforts: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, identity string
		var inLeaderboard, isLift, methodA, cableHold bool
		if err := rows.Scan(&id, &identity, &inLeaderboard, &isLift, &methodA, &cableHold); err != nil {
			t.Fatalf("scan lift effort: %v", err)
		}
		if identity == "" {
			t.Fatalf("lift effort %s has empty match identity", id)
		}
		if inLeaderboard || !isLift || !methodA || cableHold {
			t.Fatalf("lift effort %s flags = leaderboard:%v lift:%v methodA:%v cable:%v",
				id, inLeaderboard, isLift, methodA, cableHold)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("lift rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("lift effort count = %d, want 2", count)
	}
}

func TestMigration_SegmentVersionValidityAndMatchMetadata_BackfillDisambiguatesSameDayVersions(t *testing.T) {
	sql := readSegmentValidityMigrationSQL(t)
	for _, needle := range []string{
		"ROW_NUMBER() OVER",
		"INTERVAL '1 microsecond'",
		"PARTITION BY segment_id, base_from_at",
		"validity_basis = CASE",
	} {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration SQL missing same-day backfill fragment %q", needle)
		}
	}
}

func TestMigration_SegmentVersionValidityAndMatchMetadata_SupersedesExactHistoricalDuplicates(t *testing.T) {
	sql := readSegmentValidityMigrationSQL(t)
	for _, needle := range []string{
		"exact_duplicates AS",
		"duplicate_rank > 1",
		"superseded_by_effort_id = d.keep_id",
		"in_leaderboard = false",
		"ROW_NUMBER() OVER",
	} {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration SQL missing duplicate-supersede fragment %q", needle)
		}
	}
}

func TestMigration_SegmentVersionValidityAndMatchMetadata_BackfillsSegmentElevationKind(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	userID := insertSegmentValidityUser(t, ctx, pool)

	cases := []struct {
		name        string
		segmentTyp  string
		climbType   string
		requested   *string
		wantKind    string
		wantClimb   string
		expectError bool
	}{
		{name: "standard flat", segmentTyp: "standard", climbType: "standard", wantKind: "flat", wantClimb: "standard"},
		{name: "bergauf uphill", segmentTyp: "standard", climbType: "bergauf", wantKind: "uphill", wantClimb: "bergauf"},
		{name: "bergfahrt uphill", segmentTyp: "standard", climbType: "bergfahrt", wantKind: "uphill", wantClimb: "bergfahrt"},
		{name: "bergab downhill", segmentTyp: "standard", climbType: "bergab", wantKind: "downhill", wantClimb: "bergab"},
		{name: "talfahrt downhill", segmentTyp: "standard", climbType: "talfahrt", wantKind: "downhill", wantClimb: "talfahrt"},
		{name: "lap forces flat", segmentTyp: "lap", climbType: "bergauf", requested: stringPtr("hilly"), wantKind: "flat", wantClimb: "standard"},
		{name: "reject rolling", segmentTyp: "standard", climbType: "standard", requested: stringPtr("rolling"), expectError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segmentID := uuid.NewString()
			query := `
				INSERT INTO segments (id, created_by, name, sport_group, segment_type, climb_type, elevation_kind)
				VALUES ($1, $2, $3, 'bike', $4, $5, $6)
			`
			_, err := pool.Exec(ctx, query, segmentID, userID, "elevation "+tc.name, tc.segmentTyp, tc.climbType, tc.requested)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected invalid elevation kind to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("insert segment: %v", err)
			}
			var gotKind, gotClimb string
			if err := pool.QueryRow(ctx, `SELECT elevation_kind, climb_type FROM segments WHERE id = $1`, segmentID).Scan(&gotKind, &gotClimb); err != nil {
				t.Fatalf("query segment: %v", err)
			}
			if gotKind != tc.wantKind || gotClimb != tc.wantClimb {
				t.Fatalf("elevation/climb = %s/%s, want %s/%s", gotKind, gotClimb, tc.wantKind, tc.wantClimb)
			}
		})
	}

	segmentID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, climb_type)
		VALUES ($1, $2, 'update elevation mapping', 'bike', 'standard', 'standard')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert update segment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE segments
		SET elevation_kind = 'hilly'
		WHERE id = $1
	`, segmentID); err != nil {
		t.Fatalf("set hilly: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE segments
		SET description = 'preserve hilly'
		WHERE id = $1
	`, segmentID); err != nil {
		t.Fatalf("unrelated update: %v", err)
	}
	var preserved string
	if err := pool.QueryRow(ctx, `SELECT elevation_kind FROM segments WHERE id = $1`, segmentID).Scan(&preserved); err != nil {
		t.Fatalf("query preserved: %v", err)
	}
	if preserved != "hilly" {
		t.Fatalf("unrelated update elevation_kind = %s, want hilly", preserved)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE segments
		SET climb_type = 'bergab'
		WHERE id = $1
	`, segmentID); err != nil {
		t.Fatalf("change climb type: %v", err)
	}
	var remapped string
	if err := pool.QueryRow(ctx, `SELECT elevation_kind FROM segments WHERE id = $1`, segmentID).Scan(&remapped); err != nil {
		t.Fatalf("query remapped: %v", err)
	}
	if remapped != "downhill" {
		t.Fatalf("climb update elevation_kind = %s, want downhill", remapped)
	}
}

func insertSegmentValidityFixture(t *testing.T, ctx context.Context, pool execer) (userID, activityID, segmentID, versionID string) {
	t.Helper()
	userID = insertSegmentValidityUser(t, ctx, pool)
	activityID = uuid.NewString()
	startedAt := time.Date(2026, 5, 14, 9, 30, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (
			id, user_id, file_hash, title, sport_type, sub_type,
			is_indoor, is_virtual, is_electric, started_at, visibility, enrichment_status
		)
		VALUES ($1, $2, $3, 'Segment validity migration activity', 'cycling', 'mountain',
		        true, true, true, $4, 'public', 'complete')
	`, activityID, userID, "segment-validity-"+activityID, startedAt); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, activityID)
	})

	segmentID = uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, climb_type)
		VALUES ($1, $2, 'Segment validity migration segment', 'bike', 'standard', 'standard')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM segments WHERE id = $1`, segmentID)
	})

	versionID = uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, start_point, end_point,
			valid_from, valid_from_at, valid_until_at, validity_basis, created_by
		)
		VALUES (
			$1, $2, 1,
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			ST_GeomFromText('POINT(8 49)', 4326),
			ST_GeomFromText('POINT(8.01 49.01)', 4326),
			'2026-01-01',
			'2026-01-01T00:00:00Z',
			'2026-02-01T00:00:00Z',
			'editor_asserted',
			$3
		)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	return userID, activityID, segmentID, versionID
}

type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func insertSegmentValidityUser(t *testing.T, ctx context.Context, pool execer) string {
	t.Helper()
	userID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, userID, "segvalid_"+userID[:8], "segvalid_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID
}

func assertCheckRejects(t *testing.T, ctx context.Context, pool execer, query, id string) {
	t.Helper()
	tx, err := poolBegin(ctx, pool)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, query, id)
	if err == nil {
		t.Fatalf("expected CHECK rejection for %s", query)
	}
}

type beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func poolBegin(ctx context.Context, pool execer) (pgx.Tx, error) {
	return pool.(beginner).Begin(ctx)
}

func stringPtr(value string) *string {
	return &value
}
