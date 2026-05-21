package database

import (
	"context"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestTargetVisibilityAccessPairValidation(t *testing.T) {
	scopeID := uuid.NewString()

	for name, tc := range map[string]struct {
		visibility    string
		accessScopeID *string
		wantErr       bool
	}{
		"private without scope": {visibility: "private"},
		"public without scope":  {visibility: "public"},
		"shared with scope":     {visibility: "shared", accessScopeID: &scopeID},
		"shared without scope":  {visibility: "shared", wantErr: true},
		"private with scope":    {visibility: "private", accessScopeID: &scopeID, wantErr: true},
		"public with scope":     {visibility: "public", accessScopeID: &scopeID, wantErr: true},
		"unknown visibility":    {visibility: "friends", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateTargetVisibilityAccessPair(tc.visibility, tc.accessScopeID)
			if tc.wantErr && !errors.Is(err, ErrTargetValidation) {
				t.Fatalf("error = %v, want ErrTargetValidation", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)

	userScope, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "manual",
		CreatedBy:   ownerID,
		Grants: []model.ObjectAccessGrantInput{
			{PrincipalKind: "user", PrincipalUserID: &viewerID},
		},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, userScope.ID, "user", ownerID, ""); err != nil {
		t.Fatalf("user-owned scope rejected: %v", err)
	}
	if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, userScope.ID, "organization", "", "org:one"); !errors.Is(err, ErrObjectAccessNotFound) {
		t.Fatalf("organization accepted user-owned scope: %v", err)
	}

	orgScopeID := insertTargetAccessScopeFixture(t, db, "organization", "", "org:one", viewerID)
	if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, orgScopeID, "organization", "", "org:one"); err != nil {
		t.Fatalf("organization-owned scope rejected: %v", err)
	}
	if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, orgScopeID, "organization", "", "org:other"); !errors.Is(err, ErrObjectAccessNotFound) {
		t.Fatalf("organization mismatched owner ref error = %v, want ErrObjectAccessNotFound", err)
	}
	if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, orgScopeID, "system", "", "org:one"); !errors.Is(err, ErrObjectAccessNotFound) {
		t.Fatalf("system accepted organization scope: %v", err)
	}

	systemScopeID := insertTargetAccessScopeFixture(t, db, "system", "", "system:places", viewerID)
	if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, systemScopeID, "system", "", "system:places"); err != nil {
		t.Fatalf("system-owned scope rejected: %v", err)
	}
	if err := db.ValidateObjectAccessScopeForTargetOwner(ctx, systemScopeID, "user", ownerID, ""); !errors.Is(err, ErrObjectAccessNotFound) {
		t.Fatalf("user accepted system scope: %v", err)
	}
}

func TestCollectionMemberInputValidation(t *testing.T) {
	memberID := uuid.NewString()
	memberRef := "external:one"

	for name, tc := range map[string]struct {
		memberKind string
		memberID   *string
		memberRef  *string
		wantErr    bool
	}{
		"place id":                  {memberKind: "place", memberID: &memberID},
		"region id":                 {memberKind: "region", memberID: &memberID},
		"external ref":              {memberKind: "external_ref", memberRef: &memberRef},
		"place missing id":          {memberKind: "place", wantErr: true},
		"place with ref":            {memberKind: "place", memberID: &memberID, memberRef: &memberRef, wantErr: true},
		"external missing ref":      {memberKind: "external_ref", wantErr: true},
		"external with member id":   {memberKind: "external_ref", memberID: &memberID, memberRef: &memberRef, wantErr: true},
		"rule geofence unsupported": {memberKind: "rule_zone", memberID: &memberID, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateTargetMemberInput(tc.memberKind, tc.memberID, tc.memberRef)
			if tc.wantErr && !errors.Is(err, ErrTargetValidation) {
				t.Fatalf("error = %v, want ErrTargetValidation", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGovernanceRolePermissionValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		actorType   string
		permissions []string
		wantErr     bool
	}{
		"metadata editor": {
			actorType:   "user",
			permissions: []string{"edit_metadata", "link_external_refs"},
		},
		"moderation editor": {
			actorType:   "group",
			permissions: []string{"manage_reports", "resolve_claims"},
		},
		"external credit no permissions": {
			actorType: "external_party_credit",
		},
		"unknown permission": {
			actorType:   "user",
			permissions: []string{"teleport_target"},
			wantErr:     true,
		},
		"external credit with permission": {
			actorType:   "external_party_credit",
			permissions: []string{"edit_metadata"},
			wantErr:     true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateGovernancePermissions(tc.actorType, tc.permissions)
			if tc.wantErr && !errors.Is(err, ErrTargetValidation) {
				t.Fatalf("error = %v, want ErrTargetValidation", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func insertTargetAccessScopeFixture(t *testing.T, db *DB, ownerKind, ownerUserID, ownerRef, viewerID string) string {
	t.Helper()
	scopeID := uuid.NewString()
	var ownerUserArg any
	if ownerUserID != "" {
		ownerUserArg = ownerUserID
	}
	var ownerRefArg any
	if ownerRef != "" {
		ownerRefArg = ownerRef
	}
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO object_access_scopes (
			id, owner_kind, owner_user_id, owner_ref, scope_kind
		)
		VALUES ($1, $2, $3, $4, 'manual')
	`, scopeID, ownerKind, ownerUserArg, ownerRefArg); err != nil {
		t.Fatalf("insert object access scope fixture: %v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO object_access_grants (
			access_scope_id, principal_kind, principal_user_id, permission
		)
		VALUES ($1, 'user', $2, 'view')
	`, scopeID, viewerID); err != nil {
		t.Fatalf("insert object access grant fixture: %v", err)
	}
	return scopeID
}
