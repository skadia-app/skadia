package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

var (
	ErrObjectAccessValidation = errors.New("object access validation failed")
	ErrObjectAccessNotFound   = errors.New("object access scope not found")
)

func (db *DB) CreateObjectAccessScope(ctx context.Context, input model.ObjectAccessScopeInput) (*model.ObjectAccessScope, error) {
	if strings.TrimSpace(input.OwnerUserID) == "" {
		return nil, fmt.Errorf("%w: owner user is required", ErrObjectAccessValidation)
	}
	scopeKind := strings.TrimSpace(input.ScopeKind)
	if scopeKind == "" {
		scopeKind = "manual"
	}
	createdBy := strings.TrimSpace(input.CreatedBy)
	if createdBy == "" {
		createdBy = input.OwnerUserID
	}
	if len(input.Grants) == 0 {
		return nil, fmt.Errorf("%w: at least one view grant is required", ErrObjectAccessValidation)
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create object access scope: %w", err)
	}
	defer tx.Rollback(ctx)

	var label *string
	if trimmed := strings.TrimSpace(input.Label); trimmed != "" {
		label = &trimmed
	}

	scope := &model.ObjectAccessScope{}
	if err := tx.QueryRow(ctx, `
		INSERT INTO object_access_scopes (
			owner_kind, owner_user_id, scope_kind, label, created_by
		)
		VALUES ('user', $1, $2, $3, $4)
		RETURNING id, owner_kind, owner_user_id, owner_ref, scope_kind, label, created_by, created_at, revoked_at, metadata
	`, input.OwnerUserID, scopeKind, label, createdBy).Scan(
		&scope.ID, &scope.OwnerKind, &scope.OwnerUserID, &scope.OwnerRef,
		&scope.ScopeKind, &scope.Label, &scope.CreatedBy,
		&scope.CreatedAt, &scope.RevokedAt, &scope.Metadata,
	); err != nil {
		return nil, fmt.Errorf("insert object access scope: %w", err)
	}

	for _, grantInput := range input.Grants {
		grant, err := insertObjectAccessGrant(ctx, tx, scope.ID, grantInput)
		if err != nil {
			return nil, err
		}
		scope.Grants = append(scope.Grants, *grant)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create object access scope: %w", err)
	}
	return scope, nil
}

func (db *DB) ListObjectAccessScopes(ctx context.Context, ownerUserID string) ([]model.ObjectAccessScope, error) {
	if strings.TrimSpace(ownerUserID) == "" {
		return nil, fmt.Errorf("%w: owner user is required", ErrObjectAccessValidation)
	}
	rows, err := db.Pool.Query(ctx, `
		SELECT id, owner_kind, owner_user_id, owner_ref, scope_kind, label,
		       created_by, created_at, revoked_at, metadata
		FROM object_access_scopes
		WHERE owner_kind = 'user'
		  AND owner_user_id = $1
		ORDER BY created_at DESC, id DESC
	`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("list object access scopes: %w", err)
	}
	defer rows.Close()

	scopes := []model.ObjectAccessScope{}
	scopeIndex := map[string]int{}
	for rows.Next() {
		var scope model.ObjectAccessScope
		if err := rows.Scan(
			&scope.ID, &scope.OwnerKind, &scope.OwnerUserID, &scope.OwnerRef,
			&scope.ScopeKind, &scope.Label, &scope.CreatedBy,
			&scope.CreatedAt, &scope.RevokedAt, &scope.Metadata,
		); err != nil {
			return nil, err
		}
		scopeIndex[scope.ID] = len(scopes)
		scopes = append(scopes, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(scopes) == 0 {
		return scopes, nil
	}

	ids := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		ids = append(ids, scope.ID)
	}
	grantRows, err := db.Pool.Query(ctx, `
		SELECT id, access_scope_id, principal_kind, principal_user_id, principal_ref,
		       principal_selector, principal_selector_hash, link_token_hash,
		       permission, created_at, revoked_at
		FROM object_access_grants
		WHERE access_scope_id = ANY($1::uuid[])
		ORDER BY created_at ASC, id ASC
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("list object access grants: %w", err)
	}
	defer grantRows.Close()

	for grantRows.Next() {
		var grant model.ObjectAccessGrant
		if err := grantRows.Scan(
			&grant.ID, &grant.AccessScopeID, &grant.PrincipalKind,
			&grant.PrincipalUserID, &grant.PrincipalRef,
			&grant.PrincipalSelector, &grant.PrincipalSelectorHash,
			&grant.LinkTokenHash, &grant.Permission,
			&grant.CreatedAt, &grant.RevokedAt,
		); err != nil {
			return nil, err
		}
		if idx, ok := scopeIndex[grant.AccessScopeID]; ok {
			scopes[idx].Grants = append(scopes[idx].Grants, grant)
		}
	}
	if err := grantRows.Err(); err != nil {
		return nil, err
	}
	return scopes, nil
}

func (db *DB) RevokeObjectAccessScope(ctx context.Context, scopeID, ownerUserID string) error {
	if strings.TrimSpace(scopeID) == "" || strings.TrimSpace(ownerUserID) == "" {
		return fmt.Errorf("%w: scope and owner are required", ErrObjectAccessValidation)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin revoke object access scope: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE object_access_scopes
		SET revoked_at = COALESCE(revoked_at, NOW())
		WHERE id = $1
		  AND owner_kind = 'user'
		  AND owner_user_id = $2
	`, scopeID, ownerUserID)
	if err != nil {
		return fmt.Errorf("revoke object access scope: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrObjectAccessNotFound
	}
	if err := db.demoteSharedObjectsForScopeTx(ctx, tx, scopeID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit revoke object access scope: %w", err)
	}
	return nil
}

func (db *DB) RevokeObjectAccessGrants(ctx context.Context, scopeID, ownerUserID string, grantIDs []string) error {
	if strings.TrimSpace(scopeID) == "" || strings.TrimSpace(ownerUserID) == "" {
		return fmt.Errorf("%w: scope and owner are required", ErrObjectAccessValidation)
	}
	if len(grantIDs) == 0 {
		return nil
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin revoke object access grants: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE object_access_grants g
		SET revoked_at = COALESCE(g.revoked_at, NOW())
		FROM object_access_scopes s
		WHERE g.access_scope_id = s.id
		  AND g.access_scope_id = $1
		  AND s.owner_kind = 'user'
		  AND s.owner_user_id = $2
		  AND g.id = ANY($3::uuid[])
	`, scopeID, ownerUserID, grantIDs)
	if err != nil {
		return fmt.Errorf("revoke object access grants: %w", err)
	}
	if tag.RowsAffected() != int64(len(grantIDs)) {
		return ErrObjectAccessNotFound
	}
	var liveGrantCount int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM object_access_grants
		WHERE access_scope_id = $1
		  AND permission = 'view'
		  AND revoked_at IS NULL
	`, scopeID).Scan(&liveGrantCount); err != nil {
		return fmt.Errorf("count live object access grants: %w", err)
	}
	if liveGrantCount == 0 {
		if err := db.demoteSharedObjectsForScopeTx(ctx, tx, scopeID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit revoke object access grants: %w", err)
	}
	return nil
}

func (db *DB) demoteSharedObjectsForScopeTx(ctx context.Context, tx pgx.Tx, scopeID string) error {
	rows, err := tx.Query(ctx, `
		UPDATE segments
		SET visibility = 'private',
		    access_scope_id = NULL
		WHERE visibility = 'shared'
		  AND access_scope_id = $1
		RETURNING id
	`, scopeID)
	if err != nil {
		return fmt.Errorf("demote shared segments for revoked scope: %w", err)
	}
	defer rows.Close()

	var segmentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		segmentIDs = append(segmentIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(segmentIDs) == 0 {
		return nil
	}
	for _, segmentID := range segmentIDs {
		if err := syncActiveSegmentVersionEligibilitySnapshotTx(ctx, tx, segmentID); err != nil {
			return err
		}
		if err := db.RebuildSegmentCourseEligibilityProjectionTx(ctx, tx, segmentID); err != nil {
			return err
		}
		if err := syncPublishedCourseProjectionAccessTx(ctx, tx, segmentID); err != nil {
			return err
		}
	}
	return nil
}

func insertObjectAccessGrant(ctx context.Context, q Querier, scopeID string, input model.ObjectAccessGrantInput) (*model.ObjectAccessGrant, error) {
	kind := strings.TrimSpace(input.PrincipalKind)
	if kind == "" {
		return nil, fmt.Errorf("%w: principal kind is required", ErrObjectAccessValidation)
	}
	permission := strings.TrimSpace(input.Permission)
	if permission == "" {
		permission = "view"
	}
	if permission != "view" {
		return nil, fmt.Errorf("%w: object access grants are view-only", ErrObjectAccessValidation)
	}
	if err := validateObjectAccessGrantShape(kind, input); err != nil {
		return nil, err
	}

	selectorRaw, selectorHash, err := canonicalObjectAccessSelector(input.PrincipalSelector)
	if err != nil {
		return nil, err
	}
	if kind == "link_token" {
		if input.LinkTokenHash == nil || !strings.HasPrefix(*input.LinkTokenHash, "sha256:") {
			return nil, fmt.Errorf("%w: link_token grants require a token hash", ErrObjectAccessValidation)
		}
	}

	grant := &model.ObjectAccessGrant{}
	if err := q.QueryRow(ctx, `
		INSERT INTO object_access_grants (
			access_scope_id, principal_kind, principal_user_id, principal_ref,
			principal_selector, principal_selector_hash, link_token_hash, permission
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, access_scope_id, principal_kind, principal_user_id, principal_ref,
		          principal_selector, principal_selector_hash, link_token_hash,
		          permission, created_at, revoked_at
	`, scopeID, kind, input.PrincipalUserID, input.PrincipalRef, selectorRaw, selectorHash, input.LinkTokenHash, permission).Scan(
		&grant.ID, &grant.AccessScopeID, &grant.PrincipalKind,
		&grant.PrincipalUserID, &grant.PrincipalRef,
		&grant.PrincipalSelector, &grant.PrincipalSelectorHash,
		&grant.LinkTokenHash, &grant.Permission, &grant.CreatedAt, &grant.RevokedAt,
	); err != nil {
		return nil, fmt.Errorf("insert object access grant: %w", err)
	}
	return grant, nil
}

func validateObjectAccessGrantShape(kind string, input model.ObjectAccessGrantInput) error {
	switch kind {
	case "user":
		if input.PrincipalUserID == nil || strings.TrimSpace(*input.PrincipalUserID) == "" ||
			input.PrincipalRef != nil || input.LinkTokenHash != nil {
			return fmt.Errorf("%w: user grants require only principal_user_id", ErrObjectAccessValidation)
		}
	case "link_token":
		if input.LinkTokenHash == nil || strings.TrimSpace(*input.LinkTokenHash) == "" ||
			input.PrincipalUserID != nil || input.PrincipalRef != nil {
			return fmt.Errorf("%w: link_token grants require only link_token_hash", ErrObjectAccessValidation)
		}
	case "group", "team", "organization", "role", "server", "delegated_audience":
		if input.PrincipalRef == nil || strings.TrimSpace(*input.PrincipalRef) == "" ||
			input.PrincipalUserID != nil || input.LinkTokenHash != nil {
			return fmt.Errorf("%w: %s grants require only principal_ref", ErrObjectAccessValidation, kind)
		}
	case "relationship_rule":
		if input.PrincipalRef == nil || strings.TrimSpace(*input.PrincipalRef) == "" ||
			input.PrincipalUserID != nil || input.LinkTokenHash != nil || len(input.PrincipalSelector) == 0 {
			return fmt.Errorf("%w: relationship_rule grants require principal_ref and principal_selector", ErrObjectAccessValidation)
		}
	default:
		return fmt.Errorf("%w: unsupported principal kind %q", ErrObjectAccessValidation, kind)
	}
	return nil
}

func canonicalObjectAccessSelector(selector map[string]any) ([]byte, string, error) {
	if selector == nil {
		selector = map[string]any{}
	}
	raw, err := json.Marshal(selector)
	if err != nil {
		return nil, "", fmt.Errorf("%w: marshal principal selector: %v", ErrObjectAccessValidation, err)
	}
	if string(raw) == "{}" {
		return raw, "", nil
	}
	sum := sha256.Sum256(raw)
	return raw, "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (db *DB) ValidateObjectAccessScopeForShare(ctx context.Context, scopeID, ownerUserID string) error {
	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM object_access_scopes s
			WHERE s.id = $1
			  AND s.owner_kind = 'user'
			  AND s.owner_user_id = $2
			  AND s.revoked_at IS NULL
			  AND EXISTS (
			      SELECT 1
			      FROM object_access_grants g
			      WHERE g.access_scope_id = s.id
			        AND g.permission = 'view'
			        AND g.revoked_at IS NULL
			  )
		)
	`, scopeID, ownerUserID).Scan(&exists); err != nil {
		return fmt.Errorf("validate object access scope: %w", err)
	}
	if !exists {
		return ErrObjectAccessNotFound
	}
	return nil
}

func (db *DB) ResolveVisibleAccessScopes(ctx context.Context, viewerUserID string, linkTokenHash *string, dynamicRefs []model.ObjectAccessPrincipalRef) ([]string, error) {
	args := []any{}
	clauses := []string{}

	if strings.TrimSpace(viewerUserID) != "" {
		args = append(args, viewerUserID)
		clauses = append(clauses, fmt.Sprintf("(g.principal_kind = 'user' AND g.principal_user_id = $%d::uuid)", len(args)))
	}
	if linkTokenHash != nil && strings.TrimSpace(*linkTokenHash) != "" {
		args = append(args, *linkTokenHash)
		clauses = append(clauses, fmt.Sprintf("(g.principal_kind = 'link_token' AND g.link_token_hash = $%d)", len(args)))
	}
	for _, ref := range dynamicRefs {
		if ref.PrincipalKind == "" || ref.PrincipalRef == "" {
			continue
		}
		args = append(args, ref.PrincipalKind, ref.PrincipalRef, ref.PrincipalSelectorHash)
		clauses = append(clauses, fmt.Sprintf("(g.principal_kind = $%d AND g.principal_ref = $%d AND g.principal_selector_hash = $%d)", len(args)-2, len(args)-1, len(args)))
	}
	if len(clauses) == 0 {
		return nil, nil
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT g.access_scope_id
		FROM object_access_grants g
		JOIN object_access_scopes s ON s.id = g.access_scope_id
		WHERE g.permission = 'view'
		  AND g.revoked_at IS NULL
		  AND s.revoked_at IS NULL
		  AND (`+strings.Join(clauses, " OR ")+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("resolve visible access scopes: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func visibleSegmentAccessSQL(alias, userExpr, adminExpr string) string {
	return visibleSegmentAccessWithScopesSQL(alias, userExpr, adminExpr, "")
}

func visibleSegmentAccessWithScopesSQL(alias, userExpr, adminExpr, resolvedScopesExpr string) string {
	scopeClause := ""
	if strings.TrimSpace(resolvedScopesExpr) != "" {
		scopeClause = fmt.Sprintf(" OR %s.access_scope_id = ANY(%s)", alias, resolvedScopesExpr)
	}
	return fmt.Sprintf(`(
		%s.visibility = 'public'
		OR %s.created_by = %s
		OR %s = true
		OR (
			%s.visibility = 'shared'
			AND (
				EXISTS (
					SELECT 1
					FROM object_access_scopes oas
					JOIN object_access_grants oag ON oag.access_scope_id = oas.id
					WHERE oas.id = %s.access_scope_id
					  AND oas.revoked_at IS NULL
					  AND oag.revoked_at IS NULL
					  AND oag.permission = 'view'
					  AND oag.principal_kind = 'user'
					  AND oag.principal_user_id = %s
				)
				%s
			)
		)
	)`, alias, alias, userExpr, adminExpr, alias, alias, userExpr, scopeClause)
}

func sharedSegmentAccessSQL(alias, userExpr, adminExpr string) string {
	return sharedSegmentAccessWithScopesSQL(alias, userExpr, adminExpr, "")
}

func sharedSegmentAccessWithScopesSQL(alias, userExpr, adminExpr, resolvedScopesExpr string) string {
	scopeClause := ""
	if strings.TrimSpace(resolvedScopesExpr) != "" {
		scopeClause = fmt.Sprintf(" OR %s.access_scope_id = ANY(%s)", alias, resolvedScopesExpr)
	}
	return fmt.Sprintf(`(
		%s.visibility = 'shared'
		AND (
			%s.created_by = %s
			OR %s = true
			OR EXISTS (
				SELECT 1
				FROM object_access_scopes oas
				JOIN object_access_grants oag ON oag.access_scope_id = oas.id
				WHERE oas.id = %s.access_scope_id
				  AND oas.revoked_at IS NULL
				  AND oag.revoked_at IS NULL
				  AND oag.permission = 'view'
				  AND oag.principal_kind = 'user'
				  AND oag.principal_user_id = %s
			)
			%s
		)
	)`, alias, alias, userExpr, adminExpr, alias, userExpr, scopeClause)
}

func visibleSegmentExportSQL(alias, userExpr string) string {
	return fmt.Sprintf(`(
		%s.visibility = 'public'
		OR %s.created_by = %s
		OR (
			%s.visibility = 'shared'
			AND EXISTS (
				SELECT 1
				FROM object_access_scopes oas
				JOIN object_access_grants oag ON oag.access_scope_id = oas.id
				WHERE oas.id = %s.access_scope_id
				  AND oas.revoked_at IS NULL
				  AND oag.revoked_at IS NULL
				  AND oag.permission = 'view'
				  AND oag.principal_kind = 'user'
				  AND oag.principal_user_id = %s
			)
		)
	)`, alias, alias, userExpr, alias, alias, userExpr)
}

func validateSegmentAccessScopePair(visibility string, accessScopeID *string) error {
	switch visibility {
	case "", "public", "private":
		if accessScopeID != nil && *accessScopeID != "" {
			return fmt.Errorf("%w: access_scope_id is only valid for shared segments", ErrSegmentValidation)
		}
	case "shared":
		if accessScopeID == nil || strings.TrimSpace(*accessScopeID) == "" {
			return fmt.Errorf("%w: shared segments require access_scope_id", ErrSegmentValidation)
		}
	default:
		return fmt.Errorf("%w: invalid segment visibility %q", ErrSegmentValidation, visibility)
	}
	return nil
}

func (db *DB) validateSegmentAccessScopeForOwner(ctx context.Context, visibility string, accessScopeID *string, ownerUserID string) error {
	if err := validateSegmentAccessScopePair(visibility, accessScopeID); err != nil {
		return err
	}
	if visibility == "shared" {
		if err := db.ValidateObjectAccessScopeForShare(ctx, *accessScopeID, ownerUserID); err != nil {
			return fmt.Errorf("%w: %v", ErrSegmentValidation, err)
		}
	}
	return nil
}
