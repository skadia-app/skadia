package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestGovernanceScopeRoleBindingGrantsExplicitPermission(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	adminID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "governed place", "public", nil, `[8,49]`)

	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, adminID, []string{"edit_metadata"})
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "metadata")

	canEdit, err := db.CanEditTarget(ctx, adminID, "place", place.ID, "edit_metadata")
	if err != nil {
		t.Fatalf("CanEditTarget: %v", err)
	}
	if !canEdit {
		t.Fatal("explicit governance role and binding did not grant edit_metadata")
	}
}

func TestGovernanceDoesNotInferPermissionFromRegionOverlap(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	adminID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "inside region", "public", nil, `[8.005,49.005]`)
	region := createTargetRegionFixture(t, db, ownerID, "admin region", "public", nil, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "region", region.ID)
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, adminID, []string{"edit_metadata"})

	canEdit, err := db.CanEditTarget(ctx, adminID, "place", place.ID, "edit_metadata")
	if err != nil {
		t.Fatalf("CanEditTarget: %v", err)
	}
	if canEdit {
		t.Fatal("region overlap granted edit permission without explicit binding")
	}
}

func TestExternalCreditDoesNotGrantPermission(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, ownerID, "credited place", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)
	if _, err := db.AddGovernanceActorRole(ctx, ownerID, scope.ID, model.GovernanceActorRoleCreate{
		ActorType:    "external_party_credit",
		ActorRef:     objectAccessPtrString("credit:trail-crew"),
		Role:         "sponsor",
		PublicCredit: "listed",
	}); err != nil {
		t.Fatalf("AddGovernanceActorRole external credit: %v", err)
	}
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "metadata")

	targets, err := db.ListGovernanceEffectiveTargets(ctx, model.GovernanceActor{ActorType: "external_party_credit", ActorRef: objectAccessPtrString("credit:trail-crew")}, model.GovernanceEffectiveTargetFilter{})
	if err != nil {
		t.Fatalf("ListGovernanceEffectiveTargets: %v", err)
	}
	if len(targets) != 0 {
		t.Fatalf("external credit effective targets = %#v, want none", targets)
	}
}

func TestGovernanceProjectionExcludesExpiredRows(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	adminID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "expired place", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := db.AddGovernanceActorRole(ctx, ownerID, scope.ID, model.GovernanceActorRoleCreate{
		ActorType:   "user",
		ActorUserID: &adminID,
		Role:        "maintainer",
		Permissions: []string{"edit_metadata"},
		ValidUntil:  &past,
	}); err != nil {
		t.Fatalf("AddGovernanceActorRole expired: %v", err)
	}
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "metadata")

	canEdit, err := db.CanEditTarget(ctx, adminID, "place", place.ID, "edit_metadata")
	if err != nil {
		t.Fatalf("CanEditTarget: %v", err)
	}
	if canEdit {
		t.Fatal("expired governance role granted edit permission")
	}
}

func TestGovernanceMutationWritesAuditEvents(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, ownerID, "audit place", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, ownerID, []string{"manage_roles"})
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "metadata")

	var events, snapshots int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM target_change_events WHERE governance_scope_id = $1
	`, scope.ID).Scan(&events); err != nil {
		t.Fatalf("count governance events: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM target_revision_snapshots WHERE governance_scope_id = $1
	`, scope.ID).Scan(&snapshots); err != nil {
		t.Fatalf("count governance snapshots: %v", err)
	}
	if events == 0 || snapshots == 0 {
		t.Fatalf("audit rows events=%d snapshots=%d, want both non-zero", events, snapshots)
	}
}

func TestGovernanceRejectsUnauthorizedScopeBootstrap(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	otherID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "foreign place", "public", nil, `[8,49]`)
	_, err := db.CreateGovernanceScope(ctx, otherID, model.GovernanceScopeCreate{
		Kind:             "region_scope",
		AnchorTargetKind: objectAccessPtrString("place"),
		AnchorTargetID:   &place.ID,
		OwnerScope:       "user",
		OriginKey:        "manual",
	})
	if err == nil {
		t.Fatal("non-owner created governance scope for foreign target")
	}
}

func TestGovernanceRejectsRoleGrantWithoutManageRoles(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	otherID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "role place", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)

	_, err := db.AddGovernanceActorRole(ctx, otherID, scope.ID, model.GovernanceActorRoleCreate{
		ActorType:   "user",
		ActorUserID: &otherID,
		Role:        "maintainer",
		Permissions: []string{"edit_metadata"},
	})
	if err == nil {
		t.Fatal("actor without manage_roles granted governance role")
	}
}

func TestGovernanceRejectsBindingWithoutManageRoles(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	otherID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "binding place", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)

	_, err := db.AddGovernanceScopeTargetBinding(ctx, otherID, scope.ID, model.GovernanceScopeTargetBindingCreate{
		TargetKind:       "place",
		TargetID:         place.ID,
		BindingReason:    "manual_delegation",
		PermissionsScope: "metadata",
		Source:           "owner_assignment",
	})
	if err == nil {
		t.Fatal("actor without manage_roles added governance binding")
	}
}

func TestGovernanceRejectsUnknownPermissionAtDatabase(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, ownerID, "sql check", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO governance_actor_roles (
			scope_id, actor_type, actor_user_id, role, permissions, created_by
		)
		VALUES ($1, 'user', $2, 'maintainer', ARRAY['teleport_target'], $2)
	`, scope.ID, ownerID)
	if err == nil {
		t.Fatal("database accepted unknown governance permission")
	}
}

func createGovernanceScopeFixture(t *testing.T, db *DB, ownerID, targetKind, targetID string) *model.GovernanceScope {
	t.Helper()
	scope, err := db.CreateGovernanceScope(context.Background(), ownerID, model.GovernanceScopeCreate{
		Kind:             "region_scope",
		AnchorTargetKind: &targetKind,
		AnchorTargetID:   &targetID,
		OwnerScope:       "user",
		OriginKey:        "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope: %v", err)
	}
	return scope
}

func addGovernanceRoleFixture(t *testing.T, db *DB, actorID, scopeID, targetUserID string, permissions []string) *model.GovernanceActorRole {
	t.Helper()
	role, err := db.AddGovernanceActorRole(context.Background(), actorID, scopeID, model.GovernanceActorRoleCreate{
		ActorType:   "user",
		ActorUserID: &targetUserID,
		Role:        "maintainer",
		Permissions: permissions,
	})
	if err != nil {
		t.Fatalf("AddGovernanceActorRole: %v", err)
	}
	return role
}

func addGovernanceBindingFixture(t *testing.T, db *DB, actorID, scopeID, targetKind, targetID, permissionsScope string) *model.GovernanceScopeTargetBinding {
	t.Helper()
	binding, err := db.AddGovernanceScopeTargetBinding(context.Background(), actorID, scopeID, model.GovernanceScopeTargetBindingCreate{
		TargetKind:       targetKind,
		TargetID:         targetID,
		BindingReason:    "manual_delegation",
		PermissionsScope: permissionsScope,
		Source:           "owner_assignment",
	})
	if err != nil {
		t.Fatalf("AddGovernanceScopeTargetBinding: %v", err)
	}
	return binding
}
