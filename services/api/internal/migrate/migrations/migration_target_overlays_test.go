package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigration_TargetOverlays(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for table, columns := range map[string][]string{
		"target_activity_observations": {
			"id",
			"overlay_subject_kind",
			"subject_id",
			"target_version_id",
			"activity_id",
			"user_id",
			"observation_type",
			"source_type",
			"source_id",
			"source_occurrence_key",
			"source_match_id",
			"source_effort_id",
			"source_effort_started_at",
			"source_effort_segment_version_id",
			"activity_tuple",
			"stats",
			"visibility",
			"access_scope_id",
			"visibility_snapshot",
			"privacy_policy_key",
			"authorization_mode",
			"redaction_policy_key",
			"occurred_at",
			"created_at",
			"superseded_at",
		},
		"target_reports": {
			"id",
			"overlay_subject_kind",
			"subject_id",
			"target_version_id",
			"author_user_id",
			"report_type",
			"visibility",
			"access_scope_id",
			"attribution_mode",
			"applies_to",
			"condition_state",
			"condition_severity",
			"description",
			"source_activity_id",
			"source_policy_key",
			"media_refs",
			"observed_at",
			"valid_from",
			"valid_until",
			"moderation_state",
			"resolver_effect",
			"redaction_policy_key",
			"created_at",
			"updated_at",
		},
		"target_report_status_events": {
			"id",
			"report_id",
			"actor_user_id",
			"status",
			"reason",
			"created_at",
		},
		"target_user_content": {
			"id",
			"overlay_subject_kind",
			"subject_id",
			"target_version_id",
			"user_id",
			"content_kind",
			"activity_id",
			"activity_tuple",
			"payload",
			"visibility",
			"access_scope_id",
			"moderation_state",
			"created_at",
			"updated_at",
			"deleted_at",
		},
		"target_overlay_summary_projection": {
			"overlay_subject_kind",
			"subject_id",
			"target_version_id",
			"visibility_scope",
			"public_open_report_count",
			"latest_report_at",
			"current_condition_state",
			"current_condition_severity",
			"current_condition_observed_at",
			"public_recent_observation_count_30d",
			"last_observation_at",
			"public_comment_count",
			"public_rating_count",
			"public_rating_mean",
			"public_completion_count",
			"public_check_in_count",
			"updated_at",
			"source_hash",
		},
	} {
		for _, column := range columns {
			assertTargetOverlayColumnExists(t, ctx, pool, table, column)
		}
	}

	for _, index := range []string{
		"idx_target_activity_observations_subject",
		"idx_target_activity_observations_user",
		"idx_target_activity_observations_activity",
		"idx_target_reports_subject",
		"idx_target_reports_open_subject",
		"idx_target_report_status_events_report",
		"idx_target_user_content_subject",
		"idx_target_user_content_user",
		"idx_target_overlay_summary_subject",
		"idx_target_overlay_summary_updated",
	} {
		assertTargetOverlayIndexExists(t, ctx, pool, index)
	}

	userID, scopeID := insertTargetOverlayMigrationFixture(t, ctx, pool)
	subjectID := uuid.NewString()

	for _, kind := range []string{"course", "route_publication", "place", "region", "collection"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO target_reports (
				overlay_subject_kind, subject_id, author_user_id, report_type,
				visibility, access_scope_id, attribution_mode, description,
				observed_at, moderation_state
			)
			VALUES ($1, $2, $3, 'condition', 'private', NULL, 'named', 'fixture', NOW(), 'visible')
		`, kind, uuid.NewString(), userID); err != nil {
			t.Fatalf("insert allowed target report kind %s: %v", kind, err)
		}
	}

	assertTargetOverlayCheckRejects(t, ctx, pool, "invalid overlay subject kind", `
		INSERT INTO target_reports (
			overlay_subject_kind, subject_id, author_user_id, report_type,
			visibility, attribution_mode, description, observed_at, moderation_state
		)
		VALUES ('unknown', $1, $2, 'condition', 'private', 'named', 'fixture', NOW(), 'visible')
	`, subjectID, userID)

	assertTargetOverlayCheckRejects(t, ctx, pool, "shared report without access scope", `
		INSERT INTO target_reports (
			overlay_subject_kind, subject_id, author_user_id, report_type,
			visibility, attribution_mode, description, observed_at, moderation_state
		)
		VALUES ('course', $1, $2, 'condition', 'shared', 'named', 'fixture', NOW(), 'visible')
	`, subjectID, userID)

	assertTargetOverlayCheckRejects(t, ctx, pool, "private report with access scope", `
		INSERT INTO target_reports (
			overlay_subject_kind, subject_id, author_user_id, report_type,
			visibility, access_scope_id, attribution_mode, description, observed_at, moderation_state
		)
		VALUES ('course', $1, $2, 'condition', 'private', $3, 'named', 'fixture', NOW(), 'visible')
	`, subjectID, userID, scopeID)

	reportID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO target_reports (
			id, overlay_subject_kind, subject_id, author_user_id, report_type,
			visibility, attribution_mode, description, observed_at, moderation_state
		)
		VALUES ($1, 'course', $2, $3, 'condition', 'private', 'named', 'fixture', NOW(), 'visible')
	`, reportID, subjectID, userID); err != nil {
		t.Fatalf("insert report for status cascade: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO target_report_status_events (report_id, actor_user_id, status, reason)
		VALUES ($1, $2, 'open', 'created')
	`, reportID, userID); err != nil {
		t.Fatalf("insert report status event: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM target_reports WHERE id = $1`, reportID); err != nil {
		t.Fatalf("delete report: %v", err)
	}
	var statusEvents int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM target_report_status_events WHERE report_id = $1`,
		reportID,
	).Scan(&statusEvents); err != nil {
		t.Fatalf("count report status events after cascade: %v", err)
	}
	if statusEvents != 0 {
		t.Fatalf("status events after report delete = %d, want 0", statusEvents)
	}

	insertObservation := func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO target_activity_observations (
				overlay_subject_kind, subject_id, target_version_id, user_id,
				observation_type, source_type, source_id, source_occurrence_key,
				visibility, visibility_snapshot, occurred_at
			)
			VALUES ('course', $1, NULL, $2, 'matched_traversal', 'segment_effort', 'effort-1', 'once', 'private', 'private', NOW())
		`, subjectID, userID)
		return err
	}
	if err := insertObservation(); err != nil {
		t.Fatalf("insert first observation: %v", err)
	}
	if err := insertObservation(); !isUniqueViolation(err) {
		t.Fatalf("duplicate observation with null target_version_id error = %v, want unique violation", err)
	}

	versionID := uuid.NewString()
	insertVersionObservation := func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO target_activity_observations (
				overlay_subject_kind, subject_id, target_version_id, user_id,
				observation_type, source_type, source_id, source_occurrence_key,
				visibility, visibility_snapshot, occurred_at
			)
			VALUES ('course', $1, $2, $3, 'matched_traversal', 'segment_effort', 'effort-1', 'once', 'private', 'private', NOW())
		`, subjectID, versionID, userID)
		return err
	}
	if err := insertVersionObservation(); err != nil {
		t.Fatalf("insert version observation: %v", err)
	}
	if err := insertVersionObservation(); !isUniqueViolation(err) {
		t.Fatalf("duplicate observation with target_version_id error = %v, want unique violation", err)
	}
}

func TestMigration_TargetOverlaysGDPRRegistry(t *testing.T) {
	for _, table := range []string{
		"target_activity_observations",
		"target_reports",
		"target_report_status_events",
		"target_user_content",
	} {
		if got := auth.GDPRRegistry[table]; got != auth.Export {
			t.Fatalf("GDPRRegistry[%q] = %v, want Export", table, got)
		}
		if !targetOverlayStringSliceContains(auth.ExportedTables, table) {
			t.Fatalf("ExportedTables missing %q", table)
		}
	}
	if got := auth.GDPRRegistry["target_overlay_summary_projection"]; got != auth.Derived {
		t.Fatalf("GDPRRegistry[target_overlay_summary_projection] = %v, want Derived", got)
	}
}

func assertTargetOverlayColumnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = $2
		)
	`, table, column).Scan(&exists); err != nil {
		t.Fatalf("query column %s.%s: %v", table, column, err)
	}
	if !exists {
		t.Fatalf("missing column %s.%s", table, column)
	}
}

func assertTargetOverlayIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index string) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND indexname = $1
		)
	`, index).Scan(&exists); err != nil {
		t.Fatalf("query index %s: %v", index, err)
	}
	if !exists {
		t.Fatalf("missing index %s", index)
	}
}

func insertTargetOverlayMigrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	userID := uuid.NewString()
	scopeID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, userID, "target_overlay_"+userID[:8], "target_overlay_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO object_access_scopes (id, owner_kind, owner_user_id, scope_kind, label, created_by)
		VALUES ($1, 'user', $2, 'manual', 'target overlay scope', $2)
	`, scopeID, userID); err != nil {
		t.Fatalf("insert access scope fixture: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM object_access_scopes WHERE id = $1`, scopeID) //nolint:errcheck
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)                 //nolint:errcheck
	})
	return userID, scopeID
}

func assertTargetOverlayCheckRejects(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, sql string, args ...any) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("%s: begin tx: %v", name, err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, sql, args...)
	if err == nil {
		t.Fatalf("%s: expected check violation", name)
	}
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		t.Fatalf("%s: expected pg error, got %v", name, err)
	}
	if pgErr.Code != "23514" {
		t.Fatalf("%s: expected check violation, got %s (%s)", name, pgErr.Code, pgErr.Message)
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}

func targetOverlayStringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
