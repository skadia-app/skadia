package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrTargetValidation = errors.New("target validation failed")

var allowedGovernancePermissions = map[string]struct{}{
	"edit_metadata":        {},
	"edit_geometry":        {},
	"publish":              {},
	"hide_or_archive":      {},
	"disable_leaderboard":  {},
	"disable_gps_download": {},
	"manage_reports":       {},
	"manage_events":        {},
	"resolve_claims":       {},
	"manage_members":       {},
	"manage_roles":         {},
	"link_external_refs":   {},
}

var allowedCollectionMemberKinds = map[string]struct{}{
	"course":            {},
	"route_publication": {},
	"place":             {},
	"region":            {},
	"collection":        {},
	"privacy_zone":      {},
	"tag_zone":          {},
	"external_ref":      {},
}

var allowedTargetPlaceKinds = map[string]struct{}{
	"custom": {}, "private_place": {}, "poi": {}, "entrance": {}, "parking": {},
	"summit": {}, "pass": {}, "water": {}, "settlement": {}, "amenity": {},
}

var allowedTargetRegionKinds = map[string]struct{}{
	"custom": {}, "private_area": {}, "public_pool": {}, "venue_area": {},
	"privacy_zone_mirror": {}, "admin_area": {}, "protected_area": {},
}

var allowedTargetCollectionKinds = map[string]struct{}{
	"private_map": {}, "public_pool": {}, "venue_pool": {}, "route_set": {},
	"course_set": {}, "rule_set": {}, "privacy_set": {}, "custom": {},
}

var allowedTargetDisplayStyles = map[string]struct{}{
	"map_list": {}, "ordered_list": {}, "directory": {}, "compact": {},
}

var allowedTargetDefaultSorts = map[string]struct{}{
	"manual": {}, "name": {}, "distance": {}, "updated_at": {}, "role_then_manual": {},
}

var allowedTargetExternalRefVisibility = map[string]struct{}{
	"owner_only": {}, "shared_safe": {}, "public_safe": {},
}

func validateTargetVisibilityAccessPair(visibility string, accessScopeID *string) error {
	visibility = strings.TrimSpace(visibility)
	switch visibility {
	case "private", "public":
		if accessScopeID != nil && strings.TrimSpace(*accessScopeID) != "" {
			return fmt.Errorf("%w: access_scope_id is only valid for shared targets", ErrTargetValidation)
		}
	case "shared":
		if accessScopeID == nil || strings.TrimSpace(*accessScopeID) == "" {
			return fmt.Errorf("%w: shared targets require access_scope_id", ErrTargetValidation)
		}
	default:
		return fmt.Errorf("%w: invalid target visibility %q", ErrTargetValidation, visibility)
	}
	return nil
}

func validateTargetEnum(value, field string, allowed map[string]struct{}) error {
	value = strings.TrimSpace(value)
	if _, ok := allowed[value]; !ok {
		return fmt.Errorf("%w: invalid %s %q", ErrTargetValidation, field, value)
	}
	return nil
}

func validateTargetMemberInput(memberKind string, memberID *string, memberRef *string) error {
	memberKind = strings.TrimSpace(memberKind)
	if _, ok := allowedCollectionMemberKinds[memberKind]; !ok {
		return fmt.Errorf("%w: unsupported collection member kind %q", ErrTargetValidation, memberKind)
	}
	hasID := memberID != nil && strings.TrimSpace(*memberID) != ""
	hasRef := memberRef != nil && strings.TrimSpace(*memberRef) != ""
	if memberKind == "external_ref" {
		if !hasRef || hasID {
			return fmt.Errorf("%w: external_ref members require only member_ref", ErrTargetValidation)
		}
		return nil
	}
	if !hasID || hasRef {
		return fmt.Errorf("%w: %s members require only member_id", ErrTargetValidation, memberKind)
	}
	return nil
}

func validateGovernancePermissions(actorType string, permissions []string) error {
	actorType = strings.TrimSpace(actorType)
	if actorType == "external_party_credit" && len(permissions) > 0 {
		return fmt.Errorf("%w: external public-credit actors cannot hold permissions", ErrTargetValidation)
	}
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if _, ok := allowedGovernancePermissions[permission]; !ok {
			return fmt.Errorf("%w: unknown governance permission %q", ErrTargetValidation, permission)
		}
	}
	return nil
}

func visibleTargetAccessSQL(alias, ownerColumn, userExpr, adminExpr, resolvedScopesExpr string) string {
	scopeClause := ""
	if strings.TrimSpace(resolvedScopesExpr) != "" {
		scopeClause = fmt.Sprintf(" OR %s.access_scope_id = ANY(%s)", alias, resolvedScopesExpr)
	}
	return fmt.Sprintf(`(
		%s.visibility = 'public'
		OR %s.%s = %s
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
	)`, alias, alias, ownerColumn, userExpr, adminExpr, alias, alias, userExpr, scopeClause)
}

func (db *DB) ValidateObjectAccessScopeForTargetOwner(ctx context.Context, scopeID, ownerKind, ownerUserID, ownerRef string) error {
	scopeID = strings.TrimSpace(scopeID)
	ownerKind = strings.TrimSpace(ownerKind)
	ownerUserID = strings.TrimSpace(ownerUserID)
	ownerRef = strings.TrimSpace(ownerRef)
	if scopeID == "" {
		return fmt.Errorf("%w: access scope is required", ErrTargetValidation)
	}
	if ownerKind == "user" {
		if ownerUserID == "" {
			return fmt.Errorf("%w: user owner id is required", ErrTargetValidation)
		}
		return db.ValidateObjectAccessScopeForShare(ctx, scopeID, ownerUserID)
	}
	if ownerKind != "organization" && ownerKind != "system" {
		return fmt.Errorf("%w: unsupported target owner kind %q", ErrTargetValidation, ownerKind)
	}
	if ownerRef == "" {
		return fmt.Errorf("%w: owner ref is required for %s target owners", ErrTargetValidation, ownerKind)
	}

	var exists bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM object_access_scopes s
			WHERE s.id = $1
			  AND s.owner_kind = $2
			  AND s.owner_ref = $3
			  AND s.revoked_at IS NULL
			  AND EXISTS (
			      SELECT 1
			      FROM object_access_grants g
			      WHERE g.access_scope_id = s.id
			        AND g.permission = 'view'
			        AND g.revoked_at IS NULL
			  )
		)
	`, scopeID, ownerKind, ownerRef).Scan(&exists); err != nil {
		return fmt.Errorf("validate target object access scope: %w", err)
	}
	if !exists {
		return ErrObjectAccessNotFound
	}
	return nil
}
