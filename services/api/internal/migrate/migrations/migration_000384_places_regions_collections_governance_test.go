package migrations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigration000384PlacesRegionsCollectionsGovernance(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	for table, columns := range map[string][]string{
		"target_places": {
			"id",
			"owner_user_id",
			"lifecycle_owner_kind",
			"lifecycle_owner_ref",
			"name",
			"place_kind",
			"geom",
			"visibility",
			"access_scope_id",
			"origin_key",
			"external_ref_visibility",
			"metadata",
			"created_by",
			"archived_at",
		},
		"target_regions": {
			"id",
			"owner_user_id",
			"lifecycle_owner_kind",
			"lifecycle_owner_ref",
			"name",
			"region_kind",
			"geom",
			"visibility",
			"access_scope_id",
			"origin_key",
			"external_ref_visibility",
			"metadata",
			"created_by",
			"archived_at",
		},
		"target_collections": {
			"id",
			"owner_user_id",
			"lifecycle_owner_kind",
			"name",
			"collection_kind",
			"visibility",
			"access_scope_id",
			"display_style",
			"default_sort",
			"origin_key",
			"external_ref_visibility",
		},
		"target_collection_members": {
			"id",
			"collection_id",
			"member_kind",
			"member_id",
			"member_ref",
			"binding_mode",
			"pinned_target_version_id",
			"pinned_snapshot_id",
			"member_owner_scope",
			"member_visibility_snapshot",
			"membership_visibility",
			"origin_key",
			"role",
			"removed_at",
		},
		"target_collection_member_reverse_index": {
			"member_kind",
			"member_id",
			"collection_id",
			"collection_member_id",
			"contributes_bounds",
			"updated_at",
		},
		"target_object_search_projection": {
			"object_kind",
			"object_id",
			"owner_user_id",
			"lifecycle_owner_kind",
			"name",
			"object_subkind",
			"visibility",
			"access_scope_id",
			"geom",
			"bounds",
			"centroid",
			"updated_at",
		},
		"target_collection_summary_projection": {
			"collection_id",
			"owner_user_id",
			"lifecycle_owner_kind",
			"name",
			"collection_kind",
			"visibility",
			"access_scope_id",
			"bounds",
			"owner_member_count",
			"public_member_count",
			"owner_counts_by_kind",
			"public_counts_by_kind",
			"updated_at",
		},
		"governance_scopes": {
			"id",
			"kind",
			"anchor_target_kind",
			"anchor_target_id",
			"anchor_collection_id",
			"owner_scope",
			"owner_user_id",
			"owner_ref",
			"visibility",
			"origin_key",
			"metadata",
			"valid_from",
			"valid_until",
			"created_by",
			"archived_at",
		},
		"governance_actor_roles": {
			"id",
			"scope_id",
			"actor_type",
			"actor_user_id",
			"actor_ref",
			"role",
			"permissions",
			"public_credit",
			"valid_from",
			"valid_until",
			"revoked_at",
		},
		"governance_scope_target_bindings": {
			"id",
			"scope_id",
			"target_kind",
			"target_id",
			"target_version_id",
			"binding_reason",
			"permissions_scope",
			"source",
			"audit_event_id",
			"revoked_at",
		},
		"target_change_events": {
			"id",
			"subject_type",
			"subject_id",
			"target_kind",
			"target_id",
			"collection_id",
			"governance_scope_id",
			"actor_user_id",
			"action",
			"before_snapshot_id",
			"after_snapshot_id",
			"metadata",
			"created_at",
		},
		"target_revision_snapshots": {
			"id",
			"change_event_id",
			"snapshot_role",
			"subject_type",
			"subject_id",
			"target_kind",
			"target_id",
			"collection_id",
			"governance_scope_id",
			"snapshot_hash",
			"redaction_policy",
			"snapshot",
			"created_by",
			"created_at",
		},
		"governance_actor_target_projection": {
			"actor_type",
			"actor_user_id",
			"actor_ref",
			"actor_key",
			"scope_id",
			"target_kind",
			"target_id",
			"permissions",
			"permissions_scope",
			"valid_from",
			"valid_until",
			"updated_at",
		},
	} {
		for _, column := range columns {
			assertPlacesGovernanceColumnExists(t, ctx, pool, table, column)
		}
	}

	for _, index := range []string{
		"idx_target_places_geom",
		"idx_target_regions_geom",
		"idx_target_collection_members_collection",
		"idx_target_collection_member_reverse_index_collection",
		"idx_target_object_search_projection_geom",
		"idx_target_collection_summary_projection_bounds",
		"idx_governance_actor_roles_scope",
		"idx_governance_scope_target_bindings_target",
		"idx_target_change_events_target",
		"idx_governance_actor_target_projection_actor",
	} {
		assertPlacesGovernanceIndexExists(t, ctx, pool, index)
	}

	userID, accessScopeID, governanceScopeID, collectionID := insertPlacesGovernanceMigrationFixture(t, ctx, pool)

	assertPlacesGovernanceCheckRejects(t, ctx, pool, "shared place without access scope", `
		INSERT INTO target_places (
			owner_user_id, name, place_kind, geom, visibility, origin_key, created_by
		)
		VALUES (
			$1, 'Shared place without scope', 'custom',
			ST_SetSRID(ST_MakePoint(8, 49), 4326), 'shared', 'manual', $1
		)
	`, userID)
	assertPlacesGovernanceCheckRejects(t, ctx, pool, "invalid region geometry", `
		INSERT INTO target_regions (
			owner_user_id, name, region_kind, geom, origin_key, created_by
		)
		VALUES (
			$1, 'Invalid bow-tie', 'custom',
			ST_Multi(ST_GeomFromText('POLYGON((8 49, 8.01 49.01, 8.01 49, 8 49.01, 8 49))', 4326)),
			'manual', $1
		)
	`, userID)
	assertPlacesGovernanceCheckRejects(t, ctx, pool, "external credit with permissions", `
		INSERT INTO governance_actor_roles (
			scope_id, actor_type, actor_ref, role, permissions, public_credit, created_by
		)
		VALUES (
			$1, 'external_party_credit', 'sponsor:fixture', 'sponsor',
			ARRAY['edit_metadata'], 'listed', $2
		)
	`, governanceScopeID, userID)
	assertPlacesGovernanceCheckRejects(t, ctx, pool, "unknown governance permission", `
		INSERT INTO governance_actor_roles (
			scope_id, actor_type, actor_user_id, role, permissions, created_by
		)
		VALUES (
			$1, 'user', $2, 'maintainer', ARRAY['teleport_target'], $2
		)
	`, governanceScopeID, userID)
	assertPlacesGovernanceCheckRejects(t, ctx, pool, "invalid pinned membership", `
		INSERT INTO target_collection_members (
			collection_id, member_kind, member_id, binding_mode, origin_key, added_by
		)
		VALUES (
			$1, 'place', $2, 'pinned_version', 'manual', $3
		)
	`, collectionID, uuid.NewString(), userID)
	if accessScopeID == "" {
		t.Fatal("fixture access scope missing")
	}
}

func insertPlacesGovernanceMigrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (userID, accessScopeID, governanceScopeID, collectionID string) {
	t.Helper()

	userID = uuid.NewString()
	accessScopeID = uuid.NewString()
	governanceScopeID = uuid.NewString()
	collectionID = uuid.NewString()

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
		ON CONFLICT (id) DO NOTHING
	`, userID, "target_governance_"+userID[:8], "target_governance_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO object_access_scopes (id, owner_kind, owner_user_id, scope_kind, created_by)
		VALUES ($1, 'user', $2, 'manual', $2)
	`, accessScopeID, userID); err != nil {
		t.Fatalf("insert access scope: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO target_collections (
			id, owner_user_id, name, collection_kind, visibility, origin_key, created_by
		)
		VALUES ($1, $2, 'Fixture collection', 'private_map', 'private', 'manual', $2)
	`, collectionID, userID); err != nil {
		t.Fatalf("insert collection: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO governance_scopes (
			id, kind, owner_scope, owner_user_id, origin_key, created_by
		)
		VALUES ($1, 'user_space', 'user', $2, 'manual', $2)
	`, governanceScopeID, userID); err != nil {
		t.Fatalf("insert governance scope: %v", err)
	}

	return userID, accessScopeID, governanceScopeID, collectionID
}

func assertPlacesGovernanceColumnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string) {
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

func assertPlacesGovernanceIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, index string) {
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

func assertPlacesGovernanceCheckRejects(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, sql string, args ...any) {
	t.Helper()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("%s: begin tx: %v", name, err)
	}
	defer tx.Rollback(ctx)

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
