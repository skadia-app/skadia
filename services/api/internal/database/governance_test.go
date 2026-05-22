package database

import (
	"context"
	"slices"
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
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, adminID, []string{"edit_metadata", "manage_members"})
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "metadata")

	canEdit, err := db.CanEditTarget(ctx, adminID, "place", place.ID, "edit_metadata")
	if err != nil {
		t.Fatalf("CanEditTarget: %v", err)
	}
	if !canEdit {
		t.Fatal("explicit governance role and binding did not grant edit_metadata")
	}
	canManageMembers, err := db.CanEditTarget(ctx, adminID, "place", place.ID, "manage_members")
	if err != nil {
		t.Fatalf("CanEditTarget manage_members: %v", err)
	}
	if canManageMembers {
		t.Fatal("metadata-scoped governance binding granted manage_members")
	}
}

func TestGovernanceEffectivePermissionsProjectToTargetObjectResponses(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	managerID := insertObjectAccessTestUser(t, db)
	place := createTargetPlaceFixture(t, db, ownerID, "managed place", "public", nil, `[8,49]`)
	region := createTargetRegionFixture(t, db, ownerID, "managed region", "public", nil, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)
	collection := createTargetCollectionFixture(t, db, ownerID, "managed collection", "public", nil)
	scope := createGovernanceScopeFixture(t, db, ownerID, "collection", collection.ID)
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, managerID, []string{"manage_members", "manage_reports"})
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "reports")
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "region", region.ID, "reports")
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "collection", collection.ID, "membership")

	gotPlace, err := db.GetTargetPlace(ctx, managerID, place.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetPlace: %v", err)
	}
	assertExactPermissions(t, gotPlace.EffectivePermissions, "manage_reports")

	gotRegion, err := db.GetTargetRegion(ctx, managerID, region.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetRegion: %v", err)
	}
	assertExactPermissions(t, gotRegion.EffectivePermissions, "manage_reports")

	listedPlaces, err := db.ListTargetPlaces(ctx, model.ListTargetObjectsParams{UserID: managerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetPlaces: %v", err)
	}
	assertListedTargetPlacePermissions(t, listedPlaces, place.ID, "manage_reports")

	listedRegions, err := db.ListTargetRegions(ctx, model.ListTargetObjectsParams{UserID: managerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetRegions: %v", err)
	}
	assertListedTargetRegionPermissions(t, listedRegions, region.ID, "manage_reports")

	gotCollection, err := db.GetTargetCollection(ctx, managerID, collection.ID, nil, false)
	if err != nil {
		t.Fatalf("GetTargetCollection: %v", err)
	}
	assertExactPermissions(t, gotCollection.EffectivePermissions, "manage_members")

	listed, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{UserID: managerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections: %v", err)
	}
	summary := findTargetCollectionSummary(t, listed, collection.ID)
	assertExactPermissions(t, summary.EffectivePermissions, "manage_members")
}

func TestGovernanceEffectivePermissionsProjectToSegmentResponses(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	managerID := insertObjectAccessTestUser(t, db)
	segment, err := db.CreateSegment(ctx, ownerID, objectAccessSegmentCreate("managed segment", "public", nil))
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	scope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope: %v", err)
	}
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, managerID, []string{"manage_reports"})
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "course", segment.ID, "reports")

	listed, err := db.ListSegments(ctx, model.ListSegmentsParams{UserID: managerID, Limit: -1})
	if err != nil {
		t.Fatalf("ListSegments: %v", err)
	}
	found := false
	for _, listedSegment := range listed.Segments {
		if listedSegment.ID == segment.ID {
			found = true
			assertPermissions(t, listedSegment.EffectivePermissions, "manage_reports")
		}
	}
	if !found {
		t.Fatalf("ListSegments did not return segment %s", segment.ID)
	}

	got, err := db.GetSegment(ctx, segment.ID, managerID, false, false)
	if err != nil {
		t.Fatalf("GetSegment: %v", err)
	}
	if got == nil {
		t.Fatal("GetSegment returned nil")
	}
	assertPermissions(t, got.EffectivePermissions, "manage_reports")
}

func TestSegmentResponsesOnlyExposeGovernancePermissionsUsedBySegmentUI(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	managerID := insertObjectAccessTestUser(t, db)
	segment, err := db.CreateSegment(ctx, ownerID, objectAccessSegmentCreate("managed metadata segment", "public", nil))
	if err != nil {
		t.Fatalf("CreateSegment: %v", err)
	}
	scope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope: %v", err)
	}
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, managerID, []string{"edit_metadata", "manage_reports"})
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "course", segment.ID, "metadata")
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "course", segment.ID, "reports")

	got, err := db.GetSegment(ctx, segment.ID, managerID, false, false)
	if err != nil {
		t.Fatalf("GetSegment: %v", err)
	}
	assertExactPermissions(t, got.EffectivePermissions, "manage_reports")
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

func TestGovernanceUpdateWritesBeforeAndAfterAuditSnapshots(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, ownerID, "audit update place", "public", nil, `[8,49]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", place.ID)
	role := addGovernanceRoleFixture(t, db, ownerID, scope.ID, ownerID, []string{"manage_roles"})
	binding := addGovernanceBindingFixture(t, db, ownerID, scope.ID, "place", place.ID, "metadata")

	roleName := "maintainer"
	if _, err := db.UpdateGovernanceActorRole(ctx, ownerID, scope.ID, role.ID, model.GovernanceActorRoleUpdate{
		Role: &roleName,
	}); err != nil {
		t.Fatalf("UpdateGovernanceActorRole: %v", err)
	}
	permissionScope := "full"
	if _, err := db.UpdateGovernanceScopeTargetBinding(ctx, ownerID, scope.ID, binding.ID, model.GovernanceScopeTargetBindingUpdate{
		PermissionsScope: &permissionScope,
	}); err != nil {
		t.Fatalf("UpdateGovernanceScopeTargetBinding: %v", err)
	}

	for _, tc := range []struct {
		subjectType string
		subjectID   string
	}{
		{subjectType: "governance_actor_role", subjectID: role.ID},
		{subjectType: "governance_scope_target_binding", subjectID: binding.ID},
	} {
		var beforeCount, afterCount int
		if err := db.Pool.QueryRow(ctx, `
			SELECT
				COUNT(*) FILTER (WHERE s.snapshot_role = 'before'),
				COUNT(*) FILTER (WHERE s.snapshot_role = 'after')
			FROM target_change_events e
			JOIN target_revision_snapshots s ON s.change_event_id = e.id
			WHERE e.subject_type = $1 AND e.subject_id = $2
		`, tc.subjectType, tc.subjectID).Scan(&beforeCount, &afterCount); err != nil {
			t.Fatalf("count %s audit snapshots: %v", tc.subjectType, err)
		}
		if beforeCount == 0 || afterCount == 0 {
			t.Fatalf("%s audit snapshots before=%d after=%d, want both non-zero", tc.subjectType, beforeCount, afterCount)
		}
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

func TestGovernanceRejectsUnauthorizedCollectionScopeBootstrap(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	otherID := insertObjectAccessTestUser(t, db)
	collection := createTargetCollectionFixture(t, db, ownerID, "foreign collection scope", "public", nil)
	_, err := db.CreateGovernanceScope(ctx, otherID, model.GovernanceScopeCreate{
		Kind:               "collection_scope",
		AnchorCollectionID: &collection.ID,
		OwnerScope:         "user",
		OriginKey:          "manual",
	})
	if err == nil {
		t.Fatal("non-owner created governance scope for foreign collection")
	}
}

func TestGovernanceCreateUserSpaceScopeCreatesInitialOwnerRole(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	scope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope user_space: %v", err)
	}
	var count int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM governance_actor_roles
		WHERE scope_id = $1
		  AND actor_type = 'user'
		  AND actor_user_id = $2
		  AND role = 'owner'
		  AND 'manage_roles' = ANY(permissions)
		  AND revoked_at IS NULL
	`, scope.ID, ownerID).Scan(&count); err != nil {
		t.Fatalf("count initial owner role: %v", err)
	}
	if count != 1 {
		t.Fatalf("initial owner role count = %d, want 1", count)
	}
}

func TestGovernanceCreateScopeRejectsWrongKindAnchors(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, ownerID, "wrong anchor place", "public", nil, `[8,49]`)
	region := createTargetRegionFixture(t, db, ownerID, "correct anchor region", "public", nil, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)
	collection := createTargetCollectionFixture(t, db, ownerID, "wrong anchor collection", "public", nil)

	for _, tc := range []struct {
		name  string
		input model.GovernanceScopeCreate
	}{
		{
			name: "user_space_with_target_anchor",
			input: model.GovernanceScopeCreate{
				Kind:             "user_space",
				AnchorTargetKind: objectAccessPtrString("place"),
				AnchorTargetID:   &place.ID,
				OwnerScope:       "user",
				OriginKey:        "manual",
			},
		},
		{
			name: "region_scope_with_place_anchor",
			input: model.GovernanceScopeCreate{
				Kind:             "region_scope",
				AnchorTargetKind: objectAccessPtrString("place"),
				AnchorTargetID:   &place.ID,
				OwnerScope:       "user",
				OriginKey:        "manual",
			},
		},
		{
			name: "region_scope_with_collection_anchor",
			input: model.GovernanceScopeCreate{
				Kind:               "region_scope",
				AnchorCollectionID: &collection.ID,
				OwnerScope:         "user",
				OriginKey:          "manual",
			},
		},
		{
			name: "collection_scope_with_target_anchor",
			input: model.GovernanceScopeCreate{
				Kind:             "collection_scope",
				AnchorTargetKind: objectAccessPtrString("region"),
				AnchorTargetID:   &region.ID,
				OwnerScope:       "user",
				OriginKey:        "manual",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.CreateGovernanceScope(ctx, ownerID, tc.input); err == nil {
				t.Fatal("CreateGovernanceScope accepted wrong anchor")
			}
		})
	}
}

func TestGovernanceCreateCollectionAndRegionScopesRequireCorrectAnchors(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collection := createTargetCollectionFixture(t, db, ownerID, "anchored collection", "public", nil)
	region := createTargetRegionFixture(t, db, ownerID, "anchored region", "public", nil, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)

	collectionScope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:               "collection_scope",
		AnchorCollectionID: &collection.ID,
		OwnerScope:         "user",
		OriginKey:          "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope collection_scope: %v", err)
	}
	if collectionScope.AnchorCollectionID == nil || *collectionScope.AnchorCollectionID != collection.ID {
		t.Fatalf("collection anchor = %v, want %s", collectionScope.AnchorCollectionID, collection.ID)
	}

	regionScope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:             "region_scope",
		AnchorTargetKind: objectAccessPtrString("region"),
		AnchorTargetID:   &region.ID,
		OwnerScope:       "user",
		OriginKey:        "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope region_scope: %v", err)
	}
	if regionScope.AnchorTargetKind == nil || *regionScope.AnchorTargetKind != "region" || regionScope.AnchorTargetID == nil || *regionScope.AnchorTargetID != region.ID {
		t.Fatalf("region anchor kind/id = %v/%v, want region/%s", regionScope.AnchorTargetKind, regionScope.AnchorTargetID, region.ID)
	}
}

func TestGovernanceCreateOrganizationAndSystemScopesRequireAdmin(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	adminID := insertObjectAccessTestUser(t, db)
	orgRef := "org:synthetic"
	systemRef := "system:synthetic"

	if _, err := db.CreateGovernanceScope(ctx, userID, model.GovernanceScopeCreate{
		Kind:       "organization",
		OwnerScope: "global",
		OwnerRef:   &orgRef,
		OriginKey:  "manual",
	}); err == nil {
		t.Fatal("non-admin created organization governance scope")
	}
	if _, err := db.CreateGovernanceScope(ctx, userID, model.GovernanceScopeCreate{
		Kind:       "system_scope",
		OwnerScope: "system",
		OwnerRef:   &systemRef,
		OriginKey:  "manual",
	}); err == nil {
		t.Fatal("non-admin created system governance scope")
	}

	orgScope, err := db.CreateGovernanceScopeAsActor(ctx, adminID, true, model.GovernanceScopeCreate{
		Kind:       "organization",
		OwnerScope: "global",
		OwnerRef:   &orgRef,
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("admin CreateGovernanceScope organization: %v", err)
	}
	if orgScope.OwnerRef == nil || *orgScope.OwnerRef != orgRef || orgScope.OwnerUserID != nil {
		t.Fatalf("organization owner = user:%v ref:%v, want ref %s", orgScope.OwnerUserID, orgScope.OwnerRef, orgRef)
	}

	systemScope, err := db.CreateGovernanceScopeAsActor(ctx, adminID, true, model.GovernanceScopeCreate{
		Kind:       "system_scope",
		OwnerScope: "system",
		OwnerRef:   &systemRef,
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("admin CreateGovernanceScope system_scope: %v", err)
	}
	if systemScope.OwnerRef == nil || *systemScope.OwnerRef != systemRef || systemScope.OwnerUserID != nil {
		t.Fatalf("system owner = user:%v ref:%v, want ref %s", systemScope.OwnerUserID, systemScope.OwnerRef, systemRef)
	}
}

func TestGovernanceScopeManagerAuthorizationIsScopeSpecific(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	managerID := insertObjectAccessTestUser(t, db)
	newMaintainerID := insertObjectAccessTestUser(t, db)
	collection := createTargetCollectionFixture(t, db, ownerID, "manager collection", "public", nil)
	place := createTargetPlaceFixture(t, db, ownerID, "manager place", "public", nil, `[8,49]`)

	collectionScope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:               "collection_scope",
		AnchorCollectionID: &collection.ID,
		OwnerScope:         "user",
		OriginKey:          "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope collection: %v", err)
	}
	addGovernanceRoleFixture(t, db, ownerID, collectionScope.ID, managerID, []string{"manage_roles"})
	addGovernanceBindingFixture(t, db, ownerID, collectionScope.ID, "collection", collection.ID, "membership")

	placeScope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope user_space: %v", err)
	}
	addGovernanceRoleFixture(t, db, ownerID, placeScope.ID, managerID, []string{"manage_roles"})
	addGovernanceBindingFixture(t, db, ownerID, placeScope.ID, "place", place.ID, "metadata")

	if _, err := db.AddGovernanceActorRole(ctx, managerID, placeScope.ID, model.GovernanceActorRoleCreate{
		ActorType:   "user",
		ActorUserID: &newMaintainerID,
		Role:        "maintainer",
		Permissions: []string{"edit_metadata"},
	}); err != nil {
		t.Fatalf("scope-specific manager was denied: %v", err)
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

func TestGovernanceRejectsBindingToForeignTarget(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	otherID := insertObjectAccessTestUser(t, db)
	ownPlace := createTargetPlaceFixture(t, db, ownerID, "owned binding place", "public", nil, `[8,49]`)
	foreignPlace := createTargetPlaceFixture(t, db, otherID, "foreign binding place", "public", nil, `[8.1,49.1]`)
	scope := createGovernanceScopeFixture(t, db, ownerID, "place", ownPlace.ID)

	_, err := db.AddGovernanceScopeTargetBinding(ctx, ownerID, scope.ID, model.GovernanceScopeTargetBindingCreate{
		TargetKind:       "place",
		TargetID:         foreignPlace.ID,
		BindingReason:    "manual_delegation",
		PermissionsScope: "metadata",
		Source:           "owner_assignment",
	})
	if err == nil {
		t.Fatal("scope owner bound governance permissions to a foreign target")
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
	input := model.GovernanceScopeCreate{
		Kind:       "user_space",
		OwnerScope: "user",
		OriginKey:  "manual",
	}
	switch targetKind {
	case "region":
		input.Kind = "region_scope"
		input.AnchorTargetKind = &targetKind
		input.AnchorTargetID = &targetID
	case "collection":
		input.Kind = "collection_scope"
		input.AnchorCollectionID = &targetID
	}
	scope, err := db.CreateGovernanceScope(context.Background(), ownerID, input)
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

func assertPermissions(t *testing.T, got []string, want ...string) {
	t.Helper()
	for _, permission := range want {
		if !slices.Contains(got, permission) {
			t.Fatalf("permissions = %#v, want %s", got, permission)
		}
	}
}

func assertExactPermissions(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("permissions = %#v, want exactly %#v", got, want)
	}
	assertPermissions(t, got, want...)
}

func assertListedTargetPlacePermissions(t *testing.T, places []model.TargetPlace, id string, want ...string) {
	t.Helper()
	for _, place := range places {
		if place.ID == id {
			assertPermissions(t, place.EffectivePermissions, want...)
			return
		}
	}
	t.Fatalf("places did not include %s", id)
}

func assertListedTargetRegionPermissions(t *testing.T, regions []model.TargetRegion, id string, want ...string) {
	t.Helper()
	for _, region := range regions {
		if region.ID == id {
			assertPermissions(t, region.EffectivePermissions, want...)
			return
		}
	}
	t.Fatalf("regions did not include %s", id)
}
