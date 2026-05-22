package migrations_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigration_CheckpointCourses_TablesChecksAndIndexes(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for table, columns := range map[string][]string{
		"segments": {"course_kind"},
		"course_version_controls": {
			"id",
			"segment_id",
			"segment_version_id",
			"sequence_index",
			"role",
			"primitive",
			"geometry_shape",
			"label",
			"geom",
			"hit_radius_m",
			"area_radius_m",
			"required",
		},
		"checkpoint_course_effort_controls": {
			"segment_effort_id",
			"segment_effort_started_at",
			"segment_version_id",
			"control_id",
			"sequence_index",
			"entered_at",
			"activity_stream_index",
			"evidence_hash",
		},
		"checkpoint_course_projection": {
			"segment_id",
			"active_segment_version_id",
			"control_count",
			"required_control_count",
			"display_bounds",
			"control_summary",
			"control_map_features",
		},
	} {
		for _, column := range columns {
			assertCheckpointColumnExists(t, ctx, pool, table, column)
		}
	}

	for _, index := range []string{
		"idx_course_version_controls_segment_version",
		"idx_course_version_controls_geom",
		"idx_checkpoint_course_projection_bounds",
		"idx_segments_public_line_order",
		"idx_segments_owner_course_kind_order",
		"idx_segments_shared_scope_course_kind_order",
		"idx_segment_efforts_active_user_segment_counts",
	} {
		assertCheckpointIndexExists(t, ctx, pool, index)
	}

	userID, activityID, segmentID, versionID := insertCheckpointCourseMigrationFixture(t, ctx, pool)
	if userID == "" {
		t.Fatal("fixture user missing")
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, valid_from, created_by,
			matching_contract_key, matching_contract_version, valid_from_at, validity_basis
		)
		VALUES (
			$1, $2, 2, ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326),
			CURRENT_DATE, $3, 'checkpoint_sequence_v1', 1, $4, 'editor_asserted'
		)
	`, uuid.NewString(), segmentID, userID, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("checkpoint segment version rejected: %v", err)
	}

	startID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO course_version_controls (
			id, segment_id, segment_version_id, sequence_index, role, primitive,
			geometry_shape, geom, hit_radius_m, required
		)
		VALUES ($1, $2, $3, 0, 'start', 'point', 'point', ST_SetSRID(ST_MakePoint(8, 49), 4326), 30, true)
	`, startID, segmentID, versionID); err != nil {
		t.Fatalf("valid point control rejected: %v", err)
	}

	for name, sql := range map[string]string{
		"invalid role": `
			INSERT INTO course_version_controls (
				segment_id, segment_version_id, sequence_index, role, primitive,
				geometry_shape, geom, hit_radius_m, required
			)
			VALUES ($1, $2, 10, 'pause', 'point', 'point', ST_SetSRID(ST_MakePoint(8, 49), 4326), 30, true)
		`,
		"invalid primitive": `
			INSERT INTO course_version_controls (
				segment_id, segment_version_id, sequence_index, role, primitive,
				geometry_shape, geom, hit_radius_m, required
			)
			VALUES ($1, $2, 11, 'checkpoint', 'line', 'point', ST_SetSRID(ST_MakePoint(8, 49), 4326), 30, true)
		`,
		"invalid point radius": `
			INSERT INTO course_version_controls (
				segment_id, segment_version_id, sequence_index, role, primitive,
				geometry_shape, geom, hit_radius_m, required
			)
			VALUES ($1, $2, 12, 'checkpoint', 'point', 'point', ST_SetSRID(ST_MakePoint(8, 49), 4326), 0, true)
		`,
		"invalid circle radius": `
			INSERT INTO course_version_controls (
				segment_id, segment_version_id, sequence_index, role, primitive,
				geometry_shape, geom, area_radius_m, required
			)
			VALUES ($1, $2, 13, 'checkpoint', 'area', 'circle', ST_SetSRID(ST_MakePoint(8, 49), 4326), 0, true)
		`,
		"duplicate sequence": `
			INSERT INTO course_version_controls (
				segment_id, segment_version_id, sequence_index, role, primitive,
				geometry_shape, geom, hit_radius_m, required
			)
			VALUES ($1, $2, 0, 'checkpoint', 'point', 'point', ST_SetSRID(ST_MakePoint(8.01, 49.01), 4326), 30, true)
		`,
	} {
		assertCheckpointCheckRejects(t, ctx, pool, name, sql, segmentID, versionID)
	}

	effortID := uuid.NewString()
	startedAt := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_efforts (
			id, segment_id, segment_version_id, activity_id, user_id, started_at,
			elapsed_time_s, is_reverse, sport_type, sub_type,
			matching_contract_key, matching_contract_version,
			leaderboard_contract_key, leaderboard_contract_version,
			leaderboard_version_policy_key, leaderboard_direction
		)
		VALUES (
			$1, $2, $3, $4, $5, $6,
			120, false, 'cycling', 'generic',
			'checkpoint_sequence_v1', 1,
			'checkpoint_sequence_v1', 1,
			'checkpoint_sequence_current_version', 'forward'
		)
	`, effortID, segmentID, versionID, activityID, userID, startedAt); err != nil {
		t.Fatalf("checkpoint effort rejected: %v", err)
	}
}

func TestMigration_CheckpointCourses_DownRefusesLiveCheckpointData(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, _, segmentID, _ := insertCheckpointCourseMigrationFixture(t, ctx, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	downSQL, err := os.ReadFile("000382_checkpoint_courses.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(downSQL)); err == nil {
		t.Fatal("down migration succeeded with live checkpoint course data; want refusal")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
			t.Fatalf("down migration error = %v, want guard exception", err)
		}
	}
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback refused down tx: %v", err)
	}

	var courseKind string
	if err := pool.QueryRow(ctx, `SELECT course_kind FROM segments WHERE id = $1`, segmentID).Scan(&courseKind); err != nil {
		t.Fatalf("query checkpoint segment after refused down: %v", err)
	}
	if courseKind != "checkpoint_sequence" {
		t.Fatalf("course_kind after refused down = %q, want checkpoint_sequence", courseKind)
	}
}

func insertCheckpointCourseMigrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (userID, activityID, segmentID, versionID string) {
	t.Helper()

	userID = uuid.NewString()
	activityID = uuid.NewString()
	segmentID = uuid.NewString()
	versionID = uuid.NewString()

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
		ON CONFLICT (id) DO NOTHING
	`, userID, "checkpoint_"+userID[:8], "checkpoint_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at)
		VALUES ($1, $2, 'cycling', NOW())
	`, activityID, userID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility, elevation_kind, course_kind)
		VALUES ($1, $2, 'Checkpoint fixture', 'bike', 'standard', 'private', 'flat', 'checkpoint_sequence')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, valid_from, created_by,
			matching_contract_key, matching_contract_version, valid_from_at, validity_basis
		)
		VALUES (
			$1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			CURRENT_DATE, $3, 'checkpoint_sequence_v1', 1, NULL, 'unknown_start'
		)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}

	return userID, activityID, segmentID, versionID
}

func assertCheckpointColumnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
	t.Helper()
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

func assertCheckpointIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = $1)`, index).Scan(&exists); err != nil {
		t.Fatalf("query index %s: %v", index, err)
	}
	if !exists {
		t.Fatalf("missing index %s", index)
	}
}

func assertCheckpointCheckRejects(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(ctx, sql, args...)
	if err == nil {
		t.Fatalf("%s: expected check/unique rejection", name)
	}
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		t.Fatalf("%s: expected pg error, got %v", name, err)
	}
	if pgErr.Code != "23514" && pgErr.Code != "23505" {
		t.Fatalf("%s: expected check or unique violation, got %s (%s)", name, pgErr.Code, pgErr.Message)
	}
}

func assertCheckpointColumnMissingTx(t *testing.T, ctx context.Context, tx pgx.Tx, table, column string) {
	t.Helper()
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = $2
		)`, table, column).Scan(&exists); err != nil {
		t.Fatalf("query missing column %s.%s: %v", table, column, err)
	}
	if exists {
		t.Fatalf("column %s.%s still exists", table, column)
	}
}

func assertCheckpointTableMissingTx(t *testing.T, ctx context.Context, tx pgx.Tx, table string) {
	t.Helper()
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public'
			  AND c.relname = $1
			  AND c.relkind IN ('r', 'p')
		)`, table).Scan(&exists); err != nil {
		t.Fatalf("query missing table %s: %v", table, err)
	}
	if exists {
		t.Fatalf("table %s still exists", table)
	}
}
