package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

func (db *DB) CreateGovernanceScope(ctx context.Context, userID string, input model.GovernanceScopeCreate) (*model.GovernanceScope, error) {
	return db.CreateGovernanceScopeAsActor(ctx, userID, false, input)
}

func (db *DB) CreateGovernanceScopeAsActor(ctx context.Context, userID string, isAdmin bool, input model.GovernanceScopeCreate) (*model.GovernanceScope, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user is required", ErrTargetValidation)
	}
	kind := targetDefault(input.Kind, "user_space")
	ownerScope := targetDefault(input.OwnerScope, defaultGovernanceOwnerScope(kind))
	ownerRef := input.OwnerRef
	ownerUserID := &userID
	if ownerScope != "user" {
		ownerUserID = nil
	}
	if err := db.validateGovernanceScopeCreate(ctx, userID, isAdmin, kind, ownerScope, ownerRef, input); err != nil {
		return nil, err
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create governance scope: %w", err)
	}
	defer tx.Rollback(ctx)
	scope, err := scanGovernanceScope(tx.QueryRow(ctx, `
		INSERT INTO governance_scopes (
			kind, anchor_target_kind, anchor_target_id, anchor_collection_id,
			owner_scope, owner_user_id, owner_ref, visibility, origin_key,
			metadata, valid_from, valid_until, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING `+governanceScopeColumnsSQL(""),
		kind, input.AnchorTargetKind, input.AnchorTargetID, input.AnchorCollectionID,
		ownerScope, ownerUserID, ownerRef, targetVisibilityOrDefault(input.Visibility),
		targetOriginKey(input.OriginKey), targetMetadata(input.Metadata), input.ValidFrom, input.ValidUntil, userID,
	))
	if err != nil {
		return nil, fmt.Errorf("insert governance scope: %w", err)
	}
	ownerRole, err := scanGovernanceActorRole(tx.QueryRow(ctx, `
		INSERT INTO governance_actor_roles (
			scope_id, actor_type, actor_user_id, actor_ref, role, permissions,
			public_credit, valid_from, valid_until, created_by, metadata
		)
		VALUES ($1, 'user', $2, NULL, 'owner', $3, 'hidden', $4, $5, $2, '{}'::jsonb)
		RETURNING `+governanceActorRoleColumnsSQL(""),
		scope.ID, userID, governanceOwnerPermissions(), input.ValidFrom, input.ValidUntil,
	))
	if err != nil {
		return nil, fmt.Errorf("insert initial governance owner role: %w", err)
	}
	if err := writeTargetChangeEventTx(ctx, tx, "governance_scope", scope.ID, nil, nil, &scope.ID, userID, "create_governance_scope", nil, scope); err != nil {
		return nil, err
	}
	if err := writeTargetChangeEventTx(ctx, tx, "governance_actor_role", ownerRole.ID, nil, nil, &scope.ID, userID, "add_governance_role", nil, ownerRole); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create governance scope: %w", err)
	}
	return scope, nil
}

func (db *DB) validateGovernanceScopeCreate(ctx context.Context, userID string, isAdmin bool, kind, ownerScope string, ownerRef *string, input model.GovernanceScopeCreate) error {
	if err := validateGovernanceOwner(kind, ownerScope, ownerRef); err != nil {
		return err
	}
	switch kind {
	case "user_space":
		if input.AnchorTargetKind != nil || input.AnchorTargetID != nil || input.AnchorCollectionID != nil {
			return fmt.Errorf("%w: user-space governance scopes cannot be anchored", ErrTargetValidation)
		}
		return nil
	case "collection_scope":
		if input.AnchorCollectionID == nil || strings.TrimSpace(*input.AnchorCollectionID) == "" {
			return fmt.Errorf("%w: collection governance scopes require anchor_collection_id", ErrTargetValidation)
		}
		if input.AnchorTargetKind != nil || input.AnchorTargetID != nil {
			return fmt.Errorf("%w: collection governance scopes cannot use target anchors", ErrTargetValidation)
		}
		if isAdmin {
			return nil
		}
		ok, err := db.userCanManageGovernanceBindingTarget(ctx, userID, "collection", *input.AnchorCollectionID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrTargetNotFound
		}
		return nil
	case "region_scope":
		if input.AnchorTargetKind == nil || strings.TrimSpace(*input.AnchorTargetKind) != "region" || input.AnchorTargetID == nil || strings.TrimSpace(*input.AnchorTargetID) == "" {
			return fmt.Errorf("%w: region governance scopes require a region target anchor", ErrTargetValidation)
		}
		if input.AnchorCollectionID != nil {
			return fmt.Errorf("%w: region governance scopes cannot use collection anchors", ErrTargetValidation)
		}
		if isAdmin {
			return nil
		}
		ok, err := db.userCanManageGovernanceBindingTarget(ctx, userID, "region", *input.AnchorTargetID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrTargetNotFound
		}
		return nil
	case "organization", "system_scope":
		if !isAdmin {
			return ErrTargetNotFound
		}
		if input.AnchorTargetKind != nil || input.AnchorTargetID != nil || input.AnchorCollectionID != nil {
			return fmt.Errorf("%w: organization and system governance scopes cannot use target anchors", ErrTargetValidation)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported governance scope kind %q", ErrTargetValidation, kind)
	}
}

func validateGovernanceOwner(kind, ownerScope string, ownerRef *string) error {
	hasOwnerRef := ownerRef != nil && strings.TrimSpace(*ownerRef) != ""
	switch kind {
	case "user_space", "collection_scope", "region_scope":
		if ownerScope != "user" {
			return fmt.Errorf("%w: %s governance scopes require user owner_scope", ErrTargetValidation, kind)
		}
		if hasOwnerRef {
			return fmt.Errorf("%w: user-owned governance scopes cannot set owner_ref", ErrTargetValidation)
		}
	case "organization":
		if ownerScope != "shared" && ownerScope != "global" {
			return fmt.Errorf("%w: organization governance scopes require shared or global owner_scope", ErrTargetValidation)
		}
		if !hasOwnerRef {
			return fmt.Errorf("%w: organization governance scopes require owner_ref", ErrTargetValidation)
		}
	case "system_scope":
		if ownerScope != "system" {
			return fmt.Errorf("%w: system governance scopes require system owner_scope", ErrTargetValidation)
		}
		if !hasOwnerRef {
			return fmt.Errorf("%w: system governance scopes require owner_ref", ErrTargetValidation)
		}
	}
	return nil
}

func defaultGovernanceOwnerScope(kind string) string {
	if strings.TrimSpace(kind) == "system_scope" {
		return "system"
	}
	return "user"
}

func governanceOwnerPermissions() []string {
	return []string{
		"edit_metadata",
		"edit_geometry",
		"publish",
		"hide_or_archive",
		"disable_leaderboard",
		"disable_gps_download",
		"manage_reports",
		"manage_events",
		"resolve_claims",
		"manage_members",
		"manage_roles",
		"link_external_refs",
	}
}

func (db *DB) ListGovernanceScopes(ctx context.Context, userID string, isAdmin bool, filter model.GovernanceScopeFilter) ([]model.GovernanceScope, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" && !isAdmin {
		return nil, fmt.Errorf("%w: user is required", ErrTargetValidation)
	}
	args := []any{userID, isAdmin}
	query := `
		SELECT ` + governanceScopeColumnsSQL("s") + `
		FROM governance_scopes s
		WHERE s.archived_at IS NULL
		  AND (
			$2::boolean
			OR EXISTS (
				SELECT 1
				FROM governance_actor_roles r
				WHERE r.scope_id = s.id
				  AND r.actor_type = 'user'
				  AND r.actor_user_id = $1::uuid
				  AND r.revoked_at IS NULL
				  AND (r.valid_from IS NULL OR r.valid_from <= NOW())
				  AND (r.valid_until IS NULL OR r.valid_until > NOW())
			)
		  )`
	if strings.TrimSpace(filter.Kind) != "" {
		args = append(args, strings.TrimSpace(filter.Kind))
		query += fmt.Sprintf(" AND s.kind = $%d", len(args))
	}
	if strings.TrimSpace(filter.AnchorTargetKind) != "" {
		args = append(args, strings.TrimSpace(filter.AnchorTargetKind))
		query += fmt.Sprintf(" AND s.anchor_target_kind = $%d", len(args))
	}
	if strings.TrimSpace(filter.AnchorTargetID) != "" {
		args = append(args, strings.TrimSpace(filter.AnchorTargetID))
		query += fmt.Sprintf(" AND s.anchor_target_id = $%d", len(args))
	}
	if strings.TrimSpace(filter.AnchorCollectionID) != "" {
		args = append(args, strings.TrimSpace(filter.AnchorCollectionID))
		query += fmt.Sprintf(" AND s.anchor_collection_id = $%d", len(args))
	}
	if strings.TrimSpace(filter.OwnerScope) != "" {
		args = append(args, strings.TrimSpace(filter.OwnerScope))
		query += fmt.Sprintf(" AND s.owner_scope = $%d", len(args))
	}
	query += fmt.Sprintf(" ORDER BY s.created_at DESC, s.id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, targetLimit(filter.Limit), targetOffset(filter.Offset))

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list governance scopes: %w", err)
	}
	defer rows.Close()
	var out []model.GovernanceScope
	for rows.Next() {
		scope, err := scanGovernanceScope(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *scope)
	}
	return out, rows.Err()
}

func (db *DB) GetGovernanceScope(ctx context.Context, userID string, isAdmin bool, scopeID string) (*model.GovernanceScope, error) {
	userID = strings.TrimSpace(userID)
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || (userID == "" && !isAdmin) {
		return nil, fmt.Errorf("%w: user and scope are required", ErrTargetValidation)
	}
	scope, err := scanGovernanceScope(db.Pool.QueryRow(ctx, `
		SELECT `+governanceScopeColumnsSQL("s")+`
		FROM governance_scopes s
		WHERE s.id = $1
		  AND s.archived_at IS NULL
		  AND (
			$3::boolean
			OR EXISTS (
				SELECT 1
				FROM governance_actor_roles r
				WHERE r.scope_id = s.id
				  AND r.actor_type = 'user'
				  AND r.actor_user_id = $2::uuid
				  AND r.revoked_at IS NULL
				  AND (r.valid_from IS NULL OR r.valid_from <= NOW())
				  AND (r.valid_until IS NULL OR r.valid_until > NOW())
			)
		  )
	`, scopeID, userID, isAdmin))
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrTargetNotFound
		}
		return nil, fmt.Errorf("get governance scope: %w", err)
	}
	return scope, nil
}

func (db *DB) UpdateGovernanceScope(ctx context.Context, userID, scopeID string, input model.GovernanceScopeUpdate) (*model.GovernanceScope, error) {
	if err := db.requireGovernanceScopeManager(ctx, userID, scopeID); err != nil {
		return nil, err
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	current, err := scanGovernanceScope(tx.QueryRow(ctx, `SELECT `+governanceScopeColumnsSQL("s")+` FROM governance_scopes s WHERE id = $1 FOR UPDATE`, scopeID))
	if err != nil {
		return nil, err
	}
	visibility := current.Visibility
	if input.Visibility != nil {
		visibility = targetVisibilityOrDefault(*input.Visibility)
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = targetMetadata(*input.Metadata)
	}
	scope, err := scanGovernanceScope(tx.QueryRow(ctx, `
		UPDATE governance_scopes
		SET visibility = $1,
		    metadata = $2,
		    valid_from = COALESCE($3, valid_from),
		    valid_until = COALESCE($4, valid_until),
		    archived_at = COALESCE($5, archived_at)
		WHERE id = $6
		RETURNING `+governanceScopeColumnsSQL(""),
		visibility, metadata, input.ValidFrom, input.ValidUntil, input.ArchivedAt, scopeID,
	))
	if err != nil {
		return nil, err
	}
	if err := writeTargetChangeEventTx(ctx, tx, "governance_scope", scope.ID, nil, nil, &scope.ID, userID, "update_governance_scope", current, scope); err != nil {
		return nil, err
	}
	if err := rebuildGovernanceActorTargetProjectionTx(ctx, tx, scopeID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return scope, nil
}

func (db *DB) AddGovernanceActorRole(ctx context.Context, userID, scopeID string, input model.GovernanceActorRoleCreate) (*model.GovernanceActorRole, error) {
	if err := db.requireGovernanceScopeManager(ctx, userID, scopeID); err != nil {
		return nil, err
	}
	if err := validateGovernancePermissions(input.ActorType, input.Permissions); err != nil {
		return nil, err
	}
	permissions := input.Permissions
	if permissions == nil {
		permissions = []string{}
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	role, err := scanGovernanceActorRole(tx.QueryRow(ctx, `
		INSERT INTO governance_actor_roles (
			scope_id, actor_type, actor_user_id, actor_ref, role, permissions,
			public_credit, valid_from, valid_until, created_by, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+governanceActorRoleColumnsSQL(""),
		scopeID, strings.TrimSpace(input.ActorType), input.ActorUserID, input.ActorRef,
		targetDefault(input.Role, "viewer"), permissions, targetDefault(input.PublicCredit, "hidden"),
		input.ValidFrom, input.ValidUntil, userID, targetMetadata(input.Metadata),
	))
	if err != nil {
		return nil, fmt.Errorf("insert governance actor role: %w", err)
	}
	if err := rebuildGovernanceActorTargetProjectionTx(ctx, tx, scopeID); err != nil {
		return nil, err
	}
	if err := writeTargetChangeEventTx(ctx, tx, "governance_actor_role", role.ID, nil, nil, &scopeID, userID, "add_governance_role", nil, role); err != nil {
		return nil, err
	}
	return role, tx.Commit(ctx)
}

func (db *DB) UpdateGovernanceActorRole(ctx context.Context, userID, scopeID, roleID string, input model.GovernanceActorRoleUpdate) (*model.GovernanceActorRole, error) {
	if err := db.requireGovernanceScopeManager(ctx, userID, scopeID); err != nil {
		return nil, err
	}
	if len(input.Permissions) > 0 {
		if err := validateGovernancePermissions("user", input.Permissions); err != nil {
			return nil, err
		}
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	current, err := scanGovernanceActorRole(tx.QueryRow(ctx, `SELECT `+governanceActorRoleColumnsSQL("r")+` FROM governance_actor_roles r WHERE id = $1 AND scope_id = $2 FOR UPDATE`, roleID, scopeID))
	if err != nil {
		return nil, err
	}
	roleName := current.Role
	if input.Role != nil {
		roleName = strings.TrimSpace(*input.Role)
	}
	permissions := current.Permissions
	if input.Permissions != nil {
		permissions = input.Permissions
	}
	publicCredit := current.PublicCredit
	if input.PublicCredit != nil {
		publicCredit = strings.TrimSpace(*input.PublicCredit)
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = targetMetadata(*input.Metadata)
	}
	role, err := scanGovernanceActorRole(tx.QueryRow(ctx, `
		UPDATE governance_actor_roles
		SET role = $1,
		    permissions = $2,
		    public_credit = $3,
		    valid_from = COALESCE($4, valid_from),
		    valid_until = COALESCE($5, valid_until),
		    revoked_at = COALESCE($6, revoked_at),
		    metadata = $7
		WHERE id = $8 AND scope_id = $9
		RETURNING `+governanceActorRoleColumnsSQL(""),
		roleName, permissions, publicCredit, input.ValidFrom, input.ValidUntil,
		input.RevokedAt, metadata, roleID, scopeID,
	))
	if err != nil {
		return nil, err
	}
	if err := rebuildGovernanceActorTargetProjectionTx(ctx, tx, scopeID); err != nil {
		return nil, err
	}
	if err := writeTargetChangeEventTx(ctx, tx, "governance_actor_role", role.ID, nil, nil, &scopeID, userID, "update_governance_role", current, role); err != nil {
		return nil, err
	}
	return role, tx.Commit(ctx)
}

func (db *DB) AddGovernanceScopeTargetBinding(ctx context.Context, userID, scopeID string, input model.GovernanceScopeTargetBindingCreate) (*model.GovernanceScopeTargetBinding, error) {
	if err := db.requireGovernanceScopeManager(ctx, userID, scopeID); err != nil {
		return nil, err
	}
	if err := db.requireGovernanceBindingTargetAuthority(ctx, userID, input.TargetKind, input.TargetID); err != nil {
		return nil, err
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	binding, err := scanGovernanceScopeTargetBinding(tx.QueryRow(ctx, `
		INSERT INTO governance_scope_target_bindings (
			scope_id, target_kind, target_id, target_version_id, binding_reason,
			permissions_scope, source, valid_from, valid_until, created_by, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+governanceScopeTargetBindingColumnsSQL(""),
		scopeID, strings.TrimSpace(input.TargetKind), strings.TrimSpace(input.TargetID),
		input.TargetVersionID, targetDefault(input.BindingReason, "manual_delegation"),
		targetDefault(input.PermissionsScope, "metadata"), targetDefault(input.Source, "owner_assignment"),
		input.ValidFrom, input.ValidUntil, userID, targetMetadata(input.Metadata),
	))
	if err != nil {
		return nil, fmt.Errorf("insert governance target binding: %w", err)
	}
	if err := rebuildGovernanceActorTargetProjectionTx(ctx, tx, scopeID); err != nil {
		return nil, err
	}
	if err := writeTargetChangeEventTx(ctx, tx, "governance_scope_target_binding", binding.ID, &binding.TargetKind, &binding.TargetID, &scopeID, userID, "add_governance_binding", nil, binding); err != nil {
		return nil, err
	}
	return binding, tx.Commit(ctx)
}

func (db *DB) UpdateGovernanceScopeTargetBinding(ctx context.Context, userID, scopeID, bindingID string, input model.GovernanceScopeTargetBindingUpdate) (*model.GovernanceScopeTargetBinding, error) {
	if err := db.requireGovernanceScopeManager(ctx, userID, scopeID); err != nil {
		return nil, err
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	current, err := scanGovernanceScopeTargetBinding(tx.QueryRow(ctx, `SELECT `+governanceScopeTargetBindingColumnsSQL("b")+` FROM governance_scope_target_bindings b WHERE id = $1 AND scope_id = $2 FOR UPDATE`, bindingID, scopeID))
	if err != nil {
		return nil, err
	}
	permissionsScope := current.PermissionsScope
	if input.PermissionsScope != nil {
		permissionsScope = strings.TrimSpace(*input.PermissionsScope)
	}
	source := current.Source
	if input.Source != nil {
		source = strings.TrimSpace(*input.Source)
	}
	metadata := current.Metadata
	if input.Metadata != nil {
		metadata = targetMetadata(*input.Metadata)
	}
	binding, err := scanGovernanceScopeTargetBinding(tx.QueryRow(ctx, `
		UPDATE governance_scope_target_bindings
		SET target_version_id = COALESCE($1, target_version_id),
		    permissions_scope = $2,
		    source = $3,
		    valid_from = COALESCE($4, valid_from),
		    valid_until = COALESCE($5, valid_until),
		    revoked_at = COALESCE($6, revoked_at),
		    metadata = $7
		WHERE id = $8 AND scope_id = $9
		RETURNING `+governanceScopeTargetBindingColumnsSQL(""),
		input.TargetVersionID, permissionsScope, source, input.ValidFrom,
		input.ValidUntil, input.RevokedAt, metadata, bindingID, scopeID,
	))
	if err != nil {
		return nil, err
	}
	if err := rebuildGovernanceActorTargetProjectionTx(ctx, tx, scopeID); err != nil {
		return nil, err
	}
	if err := writeTargetChangeEventTx(ctx, tx, "governance_scope_target_binding", binding.ID, &binding.TargetKind, &binding.TargetID, &scopeID, userID, "update_governance_binding", current, binding); err != nil {
		return nil, err
	}
	return binding, tx.Commit(ctx)
}

func rebuildGovernanceActorTargetProjectionTx(ctx context.Context, tx pgx.Tx, scopeID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM governance_actor_target_projection WHERE scope_id = $1`, scopeID); err != nil {
		return fmt.Errorf("delete governance projection: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO governance_actor_target_projection (
			actor_type, actor_user_id, actor_ref, scope_id, target_kind, target_id,
			permissions, permissions_scope, valid_from, valid_until
		)
		SELECT r.actor_type, r.actor_user_id, r.actor_ref, r.scope_id,
		       b.target_kind, b.target_id,
		       array_agg(DISTINCT p.permission ORDER BY p.permission),
		       b.permissions_scope,
		       CASE
		           WHEN bool_and(GREATEST(r.valid_from, b.valid_from) IS NULL) THEN NULL
		           ELSE MAX(GREATEST(r.valid_from, b.valid_from))
		       END,
		       CASE
		           WHEN bool_and(LEAST(r.valid_until, b.valid_until) IS NULL) THEN NULL
		           ELSE MIN(LEAST(r.valid_until, b.valid_until))
		       END
		FROM governance_actor_roles r
		JOIN governance_scope_target_bindings b ON b.scope_id = r.scope_id
		JOIN governance_scopes s ON s.id = r.scope_id
		CROSS JOIN LATERAL unnest(r.permissions) AS p(permission)
		WHERE r.scope_id = $1
		  AND r.actor_type IN ('user', 'group', 'system')
		  AND cardinality(r.permissions) > 0
		  AND r.revoked_at IS NULL
		  AND b.revoked_at IS NULL
		  AND s.archived_at IS NULL
		  AND (r.valid_from IS NULL OR r.valid_from <= NOW())
		  AND (r.valid_until IS NULL OR r.valid_until > NOW())
		  AND (b.valid_from IS NULL OR b.valid_from <= NOW())
		  AND (b.valid_until IS NULL OR b.valid_until > NOW())
		GROUP BY r.actor_type, r.actor_user_id, r.actor_ref, r.scope_id,
		         b.target_kind, b.target_id, b.permissions_scope
	`, scopeID); err != nil {
		return fmt.Errorf("rebuild governance projection: %w", err)
	}
	return nil
}

func (db *DB) ListGovernanceEffectiveTargets(ctx context.Context, actor model.GovernanceActor, filter model.GovernanceEffectiveTargetFilter) ([]model.GovernanceActorTarget, error) {
	args := []any{strings.TrimSpace(actor.ActorType), actor.ActorUserID, actor.ActorRef}
	query := `
		SELECT actor_type, actor_user_id, actor_ref, scope_id, target_kind, target_id,
		       permissions, permissions_scope, valid_from, valid_until
		FROM governance_actor_target_projection
		WHERE actor_type = $1
		  AND (
			($2::uuid IS NOT NULL AND actor_user_id = $2::uuid)
			OR ($2::uuid IS NULL AND actor_ref = $3)
		  )
		  AND (valid_from IS NULL OR valid_from <= NOW())
		  AND (valid_until IS NULL OR valid_until > NOW())`
	if strings.TrimSpace(filter.TargetKind) != "" {
		args = append(args, strings.TrimSpace(filter.TargetKind))
		query += fmt.Sprintf(" AND target_kind = $%d", len(args))
	}
	if strings.TrimSpace(filter.TargetID) != "" {
		args = append(args, strings.TrimSpace(filter.TargetID))
		query += fmt.Sprintf(" AND target_id = $%d", len(args))
	}
	if strings.TrimSpace(filter.Permission) != "" {
		permissionArg := len(args) + 1
		args = append(args, strings.TrimSpace(filter.Permission))
		query += fmt.Sprintf(" AND $%d = ANY(permissions)", len(args))
		query += " AND " + governancePermissionScopePredicateSQL(fmt.Sprintf("$%d", permissionArg), "permissions_scope")
	}
	if strings.TrimSpace(filter.PermissionsScope) != "" {
		args = append(args, strings.TrimSpace(filter.PermissionsScope))
		query += fmt.Sprintf(" AND permissions_scope = $%d", len(args))
	}
	query += fmt.Sprintf(" ORDER BY target_kind, target_id LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, targetLimit(filter.Limit), targetOffset(filter.Offset))

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list governance effective targets: %w", err)
	}
	defer rows.Close()
	var out []model.GovernanceActorTarget
	for rows.Next() {
		var target model.GovernanceActorTarget
		if err := rows.Scan(
			&target.ActorType, &target.ActorUserID, &target.ActorRef, &target.ScopeID,
			&target.TargetKind, &target.TargetID, &target.Permissions,
			&target.PermissionsScope, &target.ValidFrom, &target.ValidUntil,
		); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}

func governanceEffectivePermissionsSQL(targetKind, targetIDExpr, actorUserExpr string) string {
	return fmt.Sprintf(`COALESCE(ARRAY(
		SELECT DISTINCT permission.permission
		FROM governance_actor_target_projection gatp
		CROSS JOIN LATERAL unnest(gatp.permissions) AS permission(permission)
		WHERE gatp.actor_type = 'user'
		  AND gatp.actor_user_id = %s
		  AND gatp.target_kind = %s
		  AND gatp.target_id = %s
		  AND (gatp.valid_from IS NULL OR gatp.valid_from <= NOW())
		  AND (gatp.valid_until IS NULL OR gatp.valid_until > NOW())
		  AND `+governancePermissionScopePredicateSQL("permission.permission", "gatp.permissions_scope")+`
		ORDER BY permission.permission
		), '{}'::text[])`, actorUserExpr, targetKindLiteral(targetKind), targetIDExpr)
}

func governanceEffectiveReportPermissionsSQL(targetKind, targetIDExpr, actorUserExpr string) string {
	return fmt.Sprintf(`COALESCE(ARRAY(
		SELECT DISTINCT permission.permission
		FROM governance_actor_target_projection gatp
		CROSS JOIN LATERAL unnest(gatp.permissions) AS permission(permission)
		WHERE gatp.actor_type = 'user'
		  AND gatp.actor_user_id = %s
		  AND gatp.target_kind = %s
		  AND gatp.target_id = %s
		  AND permission.permission = 'manage_reports'
		  AND (gatp.valid_from IS NULL OR gatp.valid_from <= NOW())
		  AND (gatp.valid_until IS NULL OR gatp.valid_until > NOW())
		  AND `+governancePermissionScopePredicateSQL("permission.permission", "gatp.permissions_scope")+`
		ORDER BY permission.permission
	), '{}'::text[])`, actorUserExpr, targetKindLiteral(targetKind), targetIDExpr)
}

func governanceTargetVisibleSQL(targetKindExpr, targetIDExpr, actorUserExpr string) string {
	return fmt.Sprintf(`EXISTS (
		SELECT 1
		FROM governance_actor_target_projection gatp
		CROSS JOIN LATERAL unnest(gatp.permissions) AS permission(permission)
		WHERE gatp.actor_type = 'user'
		  AND gatp.actor_user_id = %s
		  AND gatp.target_kind = %s
		  AND gatp.target_id = %s
		  AND (gatp.valid_from IS NULL OR gatp.valid_from <= NOW())
		  AND (gatp.valid_until IS NULL OR gatp.valid_until > NOW())
		  AND `+governancePermissionScopePredicateSQL("permission.permission", "gatp.permissions_scope")+`
	)`, actorUserExpr, targetKindExpr, targetIDExpr)
}

func governancePermissionScopePredicateSQL(permissionExpr, scopeExpr string) string {
	return fmt.Sprintf(`(
		%s = 'full'
		OR (%s = 'edit_metadata' AND %s = 'metadata')
		OR (%s = 'link_external_refs' AND %s = 'metadata')
		OR (%s = 'edit_geometry' AND %s = 'geometry')
		OR (%s = 'manage_reports' AND %s = 'reports')
		OR (%s = 'manage_events' AND %s = 'events')
		OR (%s = 'manage_members' AND %s = 'membership')
		OR (%s IN ('hide_or_archive', 'disable_leaderboard', 'disable_gps_download', 'resolve_claims') AND %s = 'moderation')
	)`,
		scopeExpr,
		permissionExpr, scopeExpr,
		permissionExpr, scopeExpr,
		permissionExpr, scopeExpr,
		permissionExpr, scopeExpr,
		permissionExpr, scopeExpr,
		permissionExpr, scopeExpr,
		permissionExpr, scopeExpr,
	)
}

func targetKindLiteral(targetKind string) string {
	switch strings.TrimSpace(targetKind) {
	case "place":
		return "'place'"
	case "region":
		return "'region'"
	case "collection":
		return "'collection'"
	case "course":
		return "'course'"
	default:
		return "''"
	}
}

func (db *DB) userOwnsTarget(ctx context.Context, userID, targetKind, targetID string) (bool, error) {
	table := governanceTargetTable(targetKind)
	if table == "" {
		return false, fmt.Errorf("%w: unsupported target kind %q", ErrTargetValidation, targetKind)
	}
	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM `+table+`
			WHERE id = $1
			  AND owner_user_id = $2
			  AND archived_at IS NULL
		)
	`, targetID, userID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (db *DB) requireGovernanceBindingTargetAuthority(ctx context.Context, userID, targetKind, targetID string) error {
	userID = strings.TrimSpace(userID)
	targetKind = strings.TrimSpace(targetKind)
	targetID = strings.TrimSpace(targetID)
	if userID == "" || targetKind == "" || targetID == "" {
		return fmt.Errorf("%w: user and target are required", ErrTargetValidation)
	}
	ok, err := db.userCanManageGovernanceBindingTarget(ctx, userID, targetKind, targetID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTargetNotFound
	}
	return nil
}

func (db *DB) userCanManageGovernanceBindingTarget(ctx context.Context, userID, targetKind, targetID string) (bool, error) {
	switch strings.TrimSpace(targetKind) {
	case "place", "region", "collection":
		owned, err := db.userOwnsTarget(ctx, userID, targetKind, targetID)
		if err != nil {
			return false, err
		}
		if owned {
			return true, nil
		}
		return db.CanEditTarget(ctx, userID, targetKind, targetID, "manage_roles")
	case "course":
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM segments
				WHERE id = $1 AND created_by = $2
			)
		`, targetID, userID).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	case "route_publication":
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM route_course_publications
				WHERE id = $1 AND actor_user_id = $2 AND retired_at IS NULL
			)
		`, targetID, userID).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	case "privacy_zone":
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM user_privacy_zones
				WHERE id = $1 AND user_id = $2
			)
		`, targetID, userID).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	case "tag_zone":
		var exists bool
		if err := db.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM tag_zones z
				JOIN tags t ON t.id = z.tag_id
				WHERE z.id = $1 AND t.user_id = $2
			)
		`, targetID, userID).Scan(&exists); err != nil {
			return false, err
		}
		return exists, nil
	default:
		return false, fmt.Errorf("%w: unsupported target kind %q", ErrTargetValidation, targetKind)
	}
}

func (db *DB) requireGovernanceScopeManager(ctx context.Context, userID, scopeID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || strings.TrimSpace(scopeID) == "" {
		return fmt.Errorf("%w: user and scope are required", ErrTargetValidation)
	}
	var canManage bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM governance_actor_roles r
			JOIN governance_scopes s ON s.id = r.scope_id
			WHERE r.scope_id = $1
			  AND r.actor_type = 'user'
			  AND r.actor_user_id = $2
			  AND 'manage_roles' = ANY(r.permissions)
			  AND r.revoked_at IS NULL
			  AND s.archived_at IS NULL
			  AND (r.valid_from IS NULL OR r.valid_from <= NOW())
			  AND (r.valid_until IS NULL OR r.valid_until > NOW())
		)
	`, scopeID, userID).Scan(&canManage); err != nil {
		return err
	}
	if canManage {
		return nil
	}
	return ErrTargetNotFound
}

func governanceTargetTable(targetKind string) string {
	switch strings.TrimSpace(targetKind) {
	case "place":
		return "target_places"
	case "region":
		return "target_regions"
	case "collection":
		return "target_collections"
	default:
		return ""
	}
}

func writeTargetChangeEventTx(ctx context.Context, tx pgx.Tx, subjectType, subjectID string, targetKind, targetID, scopeID *string, actorUserID, action string, beforeSnapshot, afterSnapshot any) error {
	var eventID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO target_change_events (
			subject_type, subject_id, target_kind, target_id, governance_scope_id,
			actor_user_id, action
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, subjectType, subjectID, targetKind, targetID, scopeID, actorUserID, action).Scan(&eventID); err != nil {
		return fmt.Errorf("insert target change event: %w", err)
	}
	if beforeSnapshot != nil {
		snapshotID, err := writeTargetRevisionSnapshotTx(ctx, tx, eventID, "before", subjectType, subjectID, targetKind, targetID, scopeID, actorUserID, beforeSnapshot)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE target_change_events SET before_snapshot_id = $1 WHERE id = $2`, snapshotID, eventID); err != nil {
			return fmt.Errorf("link target before revision snapshot: %w", err)
		}
	}
	if afterSnapshot == nil {
		return nil
	}
	snapshotID, err := writeTargetRevisionSnapshotTx(ctx, tx, eventID, "after", subjectType, subjectID, targetKind, targetID, scopeID, actorUserID, afterSnapshot)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE target_change_events SET after_snapshot_id = $1 WHERE id = $2`, snapshotID, eventID); err != nil {
		return fmt.Errorf("link target after revision snapshot: %w", err)
	}
	return nil
}

func writeTargetRevisionSnapshotTx(ctx context.Context, tx pgx.Tx, eventID, role, subjectType, subjectID string, targetKind, targetID, scopeID *string, actorUserID string, snapshot any) (string, error) {
	raw, err := json.Marshal(targetAuditSnapshot(subjectType, subjectID, targetKind, targetID, scopeID, snapshot))
	if err != nil {
		return "", fmt.Errorf("marshal target revision snapshot: %w", err)
	}
	hash := targetSnapshotHash(raw)
	var snapshotID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO target_revision_snapshots (
			change_event_id, snapshot_role, subject_type, subject_id,
			target_kind, target_id, governance_scope_id, snapshot_hash,
			redaction_policy, snapshot, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'owner_private', $9, $10)
		RETURNING id
	`, eventID, role, subjectType, subjectID, targetKind, targetID, scopeID, hash, raw, actorUserID).Scan(&snapshotID); err != nil {
		return "", fmt.Errorf("insert target revision snapshot: %w", err)
	}
	return snapshotID, nil
}

func targetAuditSnapshot(subjectType, subjectID string, targetKind, targetID, scopeID *string, snapshot any) map[string]any {
	raw, _ := json.Marshal(snapshot)
	return map[string]any{
		"schema":        "target_audit_v1",
		"subject_type":  subjectType,
		"subject_id":    subjectID,
		"target_kind":   targetKind,
		"target_id":     targetID,
		"scope_id":      scopeID,
		"snapshot_hash": targetSnapshotHash(raw),
	}
}

func targetSnapshotHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func governanceScopeColumnsSQL(alias string) string {
	prefix := targetColumnPrefix(alias)
	return prefix + `id, ` + prefix + `kind, ` + prefix + `anchor_target_kind,
		` + prefix + `anchor_target_id, ` + prefix + `anchor_collection_id,
		` + prefix + `owner_scope, ` + prefix + `owner_user_id, ` + prefix + `owner_ref,
		` + prefix + `visibility, ` + prefix + `origin_key, ` + prefix + `metadata,
		` + prefix + `valid_from, ` + prefix + `valid_until, ` + prefix + `created_by,
		` + prefix + `created_at, ` + prefix + `archived_at`
}

func governanceActorRoleColumnsSQL(alias string) string {
	prefix := targetColumnPrefix(alias)
	return prefix + `id, ` + prefix + `scope_id, ` + prefix + `actor_type,
		` + prefix + `actor_user_id, ` + prefix + `actor_ref, ` + prefix + `role,
		` + prefix + `permissions, ` + prefix + `public_credit, ` + prefix + `valid_from,
		` + prefix + `valid_until, ` + prefix + `created_by, ` + prefix + `created_at,
		` + prefix + `revoked_at, ` + prefix + `metadata`
}

func governanceScopeTargetBindingColumnsSQL(alias string) string {
	prefix := targetColumnPrefix(alias)
	return prefix + `id, ` + prefix + `scope_id, ` + prefix + `target_kind,
		` + prefix + `target_id, ` + prefix + `target_version_id, ` + prefix + `binding_reason,
		` + prefix + `permissions_scope, ` + prefix + `source, ` + prefix + `valid_from,
		` + prefix + `valid_until, ` + prefix + `audit_event_id, ` + prefix + `created_by,
		` + prefix + `created_at, ` + prefix + `revoked_at, ` + prefix + `metadata`
}

func scanGovernanceScope(row pgx.Row) (*model.GovernanceScope, error) {
	var scope model.GovernanceScope
	if err := row.Scan(
		&scope.ID, &scope.Kind, &scope.AnchorTargetKind, &scope.AnchorTargetID,
		&scope.AnchorCollectionID, &scope.OwnerScope, &scope.OwnerUserID, &scope.OwnerRef,
		&scope.Visibility, &scope.OriginKey, &scope.Metadata, &scope.ValidFrom,
		&scope.ValidUntil, &scope.CreatedBy, &scope.CreatedAt, &scope.ArchivedAt,
	); err != nil {
		return nil, err
	}
	return &scope, nil
}

func scanGovernanceActorRole(row pgx.Row) (*model.GovernanceActorRole, error) {
	var role model.GovernanceActorRole
	if err := row.Scan(
		&role.ID, &role.ScopeID, &role.ActorType, &role.ActorUserID,
		&role.ActorRef, &role.Role, &role.Permissions, &role.PublicCredit,
		&role.ValidFrom, &role.ValidUntil, &role.CreatedBy, &role.CreatedAt,
		&role.RevokedAt, &role.Metadata,
	); err != nil {
		return nil, err
	}
	return &role, nil
}

func scanGovernanceScopeTargetBinding(row pgx.Row) (*model.GovernanceScopeTargetBinding, error) {
	var binding model.GovernanceScopeTargetBinding
	if err := row.Scan(
		&binding.ID, &binding.ScopeID, &binding.TargetKind, &binding.TargetID,
		&binding.TargetVersionID, &binding.BindingReason, &binding.PermissionsScope,
		&binding.Source, &binding.ValidFrom, &binding.ValidUntil, &binding.AuditEventID,
		&binding.CreatedBy, &binding.CreatedAt, &binding.RevokedAt, &binding.Metadata,
	); err != nil {
		return nil, err
	}
	return &binding, nil
}
