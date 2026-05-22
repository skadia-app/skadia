package migrations_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigration_RoutePublicationCoursesAndVariants_TablesColumnsIndexesAndChecks(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for table, columns := range map[string][]string{
		"object_access_scopes": {
			"id",
			"owner_kind",
			"owner_user_id",
			"owner_ref",
			"scope_kind",
			"label",
			"created_by",
			"created_at",
			"revoked_at",
			"metadata",
		},
		"object_access_grants": {
			"id",
			"access_scope_id",
			"principal_kind",
			"principal_user_id",
			"principal_ref",
			"principal_selector",
			"principal_selector_hash",
			"link_token_hash",
			"permission",
			"created_at",
			"revoked_at",
			"metadata",
		},
		"segments": {
			"access_scope_id",
		},
		"planned_route_snapshots": {
			"id",
			"route_id",
			"user_id",
			"source_route_version",
			"route_name",
			"geometry",
			"waypoints",
			"segments",
			"geometry_hash",
			"snapshot_hash",
		},
		"route_publication_privacy_reviews": {
			"id",
			"route_snapshot_id",
			"snapshot_hash",
			"audience_class",
			"overall_state",
			"owner_approved_at",
			"review_hash",
			"redaction_summary",
		},
		"route_course_publications": {
			"id",
			"route_id",
			"route_snapshot_id",
			"course_segment_id",
			"course_segment_version_id",
			"publish_policy",
			"equivalence_decision",
			"visibility_snapshot",
			"access_scope_id_snapshot",
			"target_publication_id",
			"target_course_segment_id",
			"target_course_segment_version_id",
			"compatibility_evidence",
			"operation_id",
			"operation_input_hash",
		},
		"route_relationships": {
			"id",
			"source_route_id",
			"target_route_id",
			"relation_type",
			"operation_id",
			"operation_input_hash",
		},
		"route_course_relationships": {
			"id",
			"source_publication_id",
			"target_publication_id",
			"source_course_segment_id",
			"target_course_segment_id",
			"relation_type",
			"operation_id",
			"operation_input_hash",
		},
		"route_profile_snapshots": {
			"id",
			"route_snapshot_id",
			"profile_schema_key",
			"profile_schema_version",
			"source_kind",
			"source_version",
			"source_hash",
			"difficulty_scales",
			"ranges",
		},
		"route_course_profile_projection": {
			"subject_kind",
			"subject_id",
			"subject_version_id",
			"subject_version_key",
			"publication_id",
			"profile_snapshot_id",
			"visibility",
			"access_scope_id",
			"sport_type",
			"sac_max_rank",
			"mtb_max_rank",
			"via_ferrata_max_rank",
		},
		"route_publication_events": {
			"id",
			"actor_user_id",
			"operation",
			"operation_id",
			"operation_input_hash",
			"subject_kind",
			"subject_id",
			"before_snapshot_hash",
			"after_snapshot_hash",
		},
	} {
		for _, column := range columns {
			assertRoutePublicationColumnExists(t, ctx, pool, table, column)
		}
	}

	for _, index := range []string{
		"idx_object_access_grants_user_lookup",
		"idx_object_access_grants_ref_lookup",
		"idx_segments_shared_access_scope",
		"idx_planned_route_snapshots_geom",
		"idx_route_publication_privacy_reviews_snapshot",
		"idx_route_course_publications_public",
		"idx_route_course_publications_access_scope",
		"idx_route_course_publications_published_order",
		"idx_rcpp_public_search",
		"idx_rcpp_difficulty_search",
		"idx_rcpp_shared_access_scope",
		"idx_rcpp_published_course_visibility_access",
	} {
		assertRoutePublicationIndexExists(t, ctx, pool, index)
	}

	userID, otherUserID, scopeID, segmentID, versionID, snapshotID := insertRoutePublicationMigrationFixture(t, ctx, pool)

	assertRoutePublicationCheckRejects(t, ctx, pool, `
		INSERT INTO object_access_scopes (owner_kind, owner_ref, scope_kind, created_by)
		VALUES ('user', 'bad-owner-ref', 'manual', $1)
	`, userID)
	assertRoutePublicationCheckRejects(t, ctx, pool, `
		INSERT INTO object_access_grants (access_scope_id, principal_kind, principal_user_id, permission)
		VALUES ($1, 'user', $2, 'edit')
	`, scopeID, otherUserID)
	assertRoutePublicationCheckRejects(t, ctx, pool, `
		UPDATE segments SET visibility = 'shared', access_scope_id = NULL WHERE id = $1
	`, segmentID)
	assertRoutePublicationCheckRejects(t, ctx, pool, `
		UPDATE segments SET visibility = 'public', access_scope_id = $2 WHERE id = $1
	`, segmentID, scopeID)
	assertRoutePublicationCheckRejects(t, ctx, pool, `
		INSERT INTO route_course_publications (
			route_snapshot_id, course_segment_id, course_segment_version_id,
			publish_policy, equivalence_decision, visibility_snapshot,
			access_scope_id_snapshot, actor_user_id, operation_id,
			operation_input_hash, observed_route_version
		)
		VALUES ($1, $2, $3, 'new_course', 'new_group', 'public',
		        $4, $5, 'bad-public-scope', 'hash', 1)
	`, snapshotID, segmentID, versionID, scopeID, userID)
	assertRoutePublicationCheckRejects(t, ctx, pool, `
		INSERT INTO route_course_publications (
			route_snapshot_id, course_segment_id, course_segment_version_id,
			publish_policy, equivalence_decision, visibility_snapshot,
			actor_user_id, operation_id, operation_input_hash, observed_route_version
		)
		VALUES ($1, $2, $3, 'continue_equivalence_group', 'continue_group', 'private',
		        $4, 'missing-target', 'hash', 1)
	`, snapshotID, segmentID, versionID, userID)
	assertRoutePublicationCheckRejects(t, ctx, pool, `
		INSERT INTO route_publication_privacy_reviews (
			route_snapshot_id, snapshot_hash, audience_class, overall_state, review_hash
		)
		VALUES ($1, 'snapshot-hash', 'friends', 'safe', 'review-hash')
	`, snapshotID)

	if _, err := pool.Exec(ctx, `
		INSERT INTO object_access_grants (
			access_scope_id, principal_kind, principal_ref, principal_selector,
			principal_selector_hash, permission
		)
		VALUES
			($1, 'relationship_rule', 'friend-depth', '{"relationship":"friend","max_depth":2}', 'selector-hash', 'view'),
			($1, 'group', 'group-1', '{}', '', 'view'),
			($1, 'team', 'team-1', '{}', '', 'view'),
			($1, 'server', 'server-1', '{}', '', 'view'),
			($1, 'delegated_audience', 'region-viewers', '{}', '', 'view')
	`, scopeID); err != nil {
		t.Fatalf("insert dynamic grants: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO object_access_grants (
			access_scope_id, principal_kind, link_token_hash, permission
		)
		VALUES ($1, 'link_token', 'sha256:token-hash', 'view')
	`, scopeID); err != nil {
		t.Fatalf("insert link-token grant: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE segments SET visibility = 'shared', access_scope_id = $2 WHERE id = $1
	`, segmentID, scopeID); err != nil {
		t.Fatalf("set segment shared: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO route_course_publications (
			route_snapshot_id, course_segment_id, course_segment_version_id,
			publish_policy, equivalence_decision, visibility_snapshot,
			access_scope_id_snapshot, actor_user_id, operation_id,
			operation_input_hash, observed_route_version
		)
		VALUES ($1, $2, $3, 'new_course', 'new_group', 'shared',
		        $4, $5, 'valid-shared', 'hash', 1)
	`, snapshotID, segmentID, versionID, scopeID, userID); err != nil {
		t.Fatalf("insert shared publication: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO route_publication_privacy_reviews (
			route_snapshot_id, snapshot_hash, audience_class,
			route_name_state, description_state, waypoints_state,
			source_metadata_state, geometry_state, enrichment_state,
			media_pointer_state, overall_state, review_hash
		)
		VALUES ($1, 'snapshot-hash', 'public',
		        'safe', 'requires_confirmation', 'safe',
		        'safe', 'safe', 'safe',
		        'safe', 'confirmed', 'review-hash')
	`, snapshotID); err != nil {
		t.Fatalf("insert privacy review: %v", err)
	}
}

func TestMigration_RoutePublicationCoursesAndVariants_DownRefusesLiveSharedPublicationData(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	_, _, scopeID, segmentID, _, _ := insertRoutePublicationMigrationFixture(t, ctx, pool)

	if _, err := pool.Exec(ctx, `UPDATE segments SET visibility = 'shared', access_scope_id = $2 WHERE id = $1`, segmentID, scopeID); err != nil {
		t.Fatalf("set segment shared before down: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	downSQL, err := os.ReadFile("000381_route_publication_courses_and_variants.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(downSQL)); err == nil {
		t.Fatal("down migration succeeded with live shared/publication data; want refusal")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "P0001" {
			t.Fatalf("down migration error = %v, want guard exception", err)
		}
	}
	if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback refused down tx: %v", err)
	}

	var stillExists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM object_access_scopes WHERE id = $1)`, scopeID).Scan(&stillExists); err != nil {
		t.Fatalf("query access scope after refused down: %v", err)
	}
	if !stillExists {
		t.Fatal("guarded down migration removed live access scope before refusing")
	}
}

func insertRoutePublicationMigrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (userID, otherUserID, scopeID, segmentID, versionID, snapshotID string) {
	t.Helper()

	userID = uuid.NewString()
	otherUserID = uuid.NewString()
	segmentID = uuid.NewString()
	versionID = uuid.NewString()
	snapshotID = uuid.NewString()
	scopeID = uuid.NewString()

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES
			($1, $2, $3, true, 'test'),
			($4, $5, $6, true, 'test')
		ON CONFLICT (id) DO NOTHING
	`, userID, "route_pub_"+userID[:8], "route_pub_"+userID[:8]+"@example.com", otherUserID, "route_pub_"+otherUserID[:8], "route_pub_"+otherUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, userID, otherUserID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO object_access_scopes (id, owner_kind, owner_user_id, scope_kind, created_by)
		VALUES ($1, 'user', $2, 'manual', $2)
	`, scopeID, userID); err != nil {
		t.Fatalf("insert access scope: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO object_access_grants (access_scope_id, principal_kind, principal_user_id, permission)
		VALUES ($1, 'user', $2, 'view')
	`, scopeID, otherUserID); err != nil {
		t.Fatalf("insert direct grant: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility, elevation_kind)
		VALUES ($1, $2, 'Route publication fixture', 'bike', 'standard', 'private', 'flat')
	`, segmentID, userID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (
			id, segment_id, version_number, track, valid_from, created_by,
			valid_from_at, validity_basis
		)
		VALUES (
			$1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			CURRENT_DATE, $3, NULL, 'unknown_start'
		)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO planned_route_snapshots (
			id, user_id, source_route_version, route_name, route_description,
			sport_type, sub_type, profile, route_schema_version, geometry,
			waypoints, segments, geometry_hash, snapshot_hash
		)
		VALUES (
			$1, $2, 1, 'Snapshot fixture', 'Fixture',
			'cycling', 'mountain', 'default', 1,
			ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
			'[]'::jsonb, '[]'::jsonb, 'geometry-hash', 'snapshot-hash'
		)
	`, snapshotID, userID); err != nil {
		t.Fatalf("insert route snapshot: %v", err)
	}

	return userID, otherUserID, scopeID, segmentID, versionID, snapshotID
}

func assertRoutePublicationColumnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
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

func assertRoutePublicationColumnMissingTx(t *testing.T, ctx context.Context, tx pgx.Tx, table, column string) {
	t.Helper()
	var exists bool
	if err := tx.QueryRow(ctx, `
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
	if exists {
		t.Fatalf("column %s.%s still exists", table, column)
	}
}

func assertRoutePublicationTableMissingTx(t *testing.T, ctx context.Context, tx pgx.Tx, table string) {
	t.Helper()
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name = $1
		)
	`, table).Scan(&exists); err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	if exists {
		t.Fatalf("table %s still exists", table)
	}
}

func assertRoutePublicationIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index string) {
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

func assertRoutePublicationCheckRejects(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := pool.Exec(ctx, sql, args...)
	if err == nil {
		t.Fatalf("expected check violation for query: %s", sql)
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); !ok || pgErr.Code != "23514" {
		t.Fatalf("error = %v, want check violation", err)
	}
}
