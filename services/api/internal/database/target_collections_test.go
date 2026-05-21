package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestCreateListTargetCollectionsWithAccessScopes(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	scope := createTargetObjectAccessScope(t, db, ownerID, viewerID)

	privateCollection := createTargetCollectionFixture(t, db, ownerID, "private map", "private", nil)
	publicCollection := createTargetCollectionFixture(t, db, ownerID, "public pool", "public", nil)
	sharedCollection := createTargetCollectionFixture(t, db, ownerID, "shared map", "shared", &scope.ID)

	ownerCollections, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{UserID: ownerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections owner: %v", err)
	}
	assertTargetCollectionIDs(t, ownerCollections, privateCollection.ID, publicCollection.ID, sharedCollection.ID)

	publicCollections, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections public: %v", err)
	}
	assertTargetCollectionIDs(t, publicCollections, publicCollection.ID)
	assertTargetCollectionIDsAbsent(t, publicCollections, privateCollection.ID, sharedCollection.ID)

	viewerCollections, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{
		UserID:                viewerID,
		VisibleAccessScopeIDs: []string{scope.ID},
		Limit:                 20,
	})
	if err != nil {
		t.Fatalf("ListTargetCollections viewer: %v", err)
	}
	assertTargetCollectionIDs(t, viewerCollections, publicCollection.ID, sharedCollection.ID)
	assertTargetCollectionIDsAbsent(t, viewerCollections, privateCollection.ID)
}

func TestCollectionMembersAuthorizeCollectionAndMemberSeparately(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	publicCollection := createTargetCollectionFixture(t, db, ownerID, "public with private member", "public", nil)
	privatePlace := createTargetPlaceFixture(t, db, ownerID, "private member", "private", nil, `[8,49]`)
	publicPlace := createTargetPlaceFixture(t, db, ownerID, "public member", "public", nil, `[8.001,49.001]`)

	if _, err := db.AddTargetCollectionMember(ctx, ownerID, publicCollection.ID, model.TargetCollectionMemberCreate{
		MemberKind: "place",
		MemberID:   &privatePlace.ID,
		OriginKey:  "manual",
		Role:       "poi",
	}); err != nil {
		t.Fatalf("AddTargetCollectionMember private: %v", err)
	}
	if _, err := db.AddTargetCollectionMember(ctx, ownerID, publicCollection.ID, model.TargetCollectionMemberCreate{
		MemberKind: "place",
		MemberID:   &publicPlace.ID,
		OriginKey:  "manual",
		Role:       "poi",
	}); err != nil {
		t.Fatalf("AddTargetCollectionMember public: %v", err)
	}

	publicDetail, err := db.GetTargetCollection(ctx, "", publicCollection.ID, nil, true)
	if err != nil {
		t.Fatalf("GetTargetCollection public: %v", err)
	}
	if len(publicDetail.Members) != 1 || publicDetail.Members[0].MemberID == nil || *publicDetail.Members[0].MemberID != publicPlace.ID {
		t.Fatalf("public members = %#v, want only public member %s", publicDetail.Members, publicPlace.ID)
	}
	if publicDetail.HiddenMemberCount != 0 {
		t.Fatalf("public detail leaked hidden member count %d", publicDetail.HiddenMemberCount)
	}

	ownerDetail, err := db.GetTargetCollection(ctx, ownerID, publicCollection.ID, nil, true)
	if err != nil {
		t.Fatalf("GetTargetCollection owner: %v", err)
	}
	if len(ownerDetail.Members) != 2 {
		t.Fatalf("owner members = %#v, want both members", ownerDetail.Members)
	}
	if ownerDetail.HiddenMemberCount == 0 {
		t.Fatal("owner detail did not expose management hidden count")
	}
}

func TestCollectionSummaryProjectionTracksBoundsAndCounts(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collection := createTargetCollectionFixture(t, db, ownerID, "bounds", "public", nil)
	place := createTargetPlaceFixture(t, db, ownerID, "bounds place", "public", nil, `[8,49]`)
	region := createTargetRegionFixture(t, db, ownerID, "bounds region", "public", nil, `[[[8.01,49.01],[8.02,49.01],[8.02,49.02],[8.01,49.02],[8.01,49.01]]]`)

	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", place.ID, "poi")
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "region", region.ID, "area")

	summaries, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections: %v", err)
	}
	summary := findTargetCollectionSummary(t, summaries, collection.ID)
	if summary.OwnerMemberCount != 2 || summary.PublicMemberCount != 2 {
		t.Fatalf("summary counts owner=%d public=%d, want 2/2", summary.OwnerMemberCount, summary.PublicMemberCount)
	}
	if summary.BoundsGeoJSON == nil || *summary.BoundsGeoJSON == "" {
		t.Fatal("summary bounds missing")
	}
}

func TestCollectionSummaryInvalidatesWhenMemberChanges(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collection := createTargetCollectionFixture(t, db, ownerID, "invalidation", "public", nil)
	place := createTargetPlaceFixture(t, db, ownerID, "mutable place", "public", nil, `[8,49]`)
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", place.ID, "poi")

	privateVisibility := "private"
	if _, err := db.UpdateTargetPlace(ctx, ownerID, place.ID, model.TargetPlaceUpdate{Visibility: &privateVisibility}); err != nil {
		t.Fatalf("UpdateTargetPlace private: %v", err)
	}
	summaries, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections after update: %v", err)
	}
	summary := findTargetCollectionSummary(t, summaries, collection.ID)
	if summary.OwnerMemberCount != 1 || summary.PublicMemberCount != 0 {
		t.Fatalf("summary counts after member visibility change owner=%d public=%d, want 1/0", summary.OwnerMemberCount, summary.PublicMemberCount)
	}
}

func TestCollectionRejectsRecursiveMembershipDepth(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	parent := createTargetCollectionFixture(t, db, ownerID, "parent", "private", nil)
	child := createTargetCollectionFixture(t, db, ownerID, "child", "private", nil)

	addTargetCollectionMemberFixture(t, db, ownerID, parent.ID, "collection", child.ID, "member")
	_, err := db.AddTargetCollectionMember(ctx, ownerID, child.ID, model.TargetCollectionMemberCreate{
		MemberKind: "collection",
		MemberID:   &parent.ID,
		OriginKey:  "manual",
	})
	if err == nil {
		t.Fatal("recursive collection membership was accepted")
	}
}

func TestCollectionMembershipDoesNotGrantEditAuthority(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	otherID := insertObjectAccessTestUser(t, db)
	collection := createTargetCollectionFixture(t, db, ownerID, "edit boundary", "public", nil)
	place := createTargetPlaceFixture(t, db, otherID, "other place", "public", nil, `[8,49]`)
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", place.ID, "poi")

	canEdit, err := db.CanEditTarget(ctx, ownerID, "place", place.ID, "edit_metadata")
	if err != nil {
		t.Fatalf("CanEditTarget: %v", err)
	}
	if canEdit {
		t.Fatal("collection membership granted edit authority on member target")
	}
}

func TestCollectionMembershipBindingModesAreEnforced(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collection := createTargetCollectionFixture(t, db, ownerID, "binding", "private", nil)
	place := createTargetPlaceFixture(t, db, ownerID, "binding place", "private", nil, `[8,49]`)

	bindingMode := "pinned_version"
	_, err := db.AddTargetCollectionMember(ctx, ownerID, collection.ID, model.TargetCollectionMemberCreate{
		MemberKind:  "place",
		MemberID:    &place.ID,
		BindingMode: &bindingMode,
		OriginKey:   "manual",
	})
	if err == nil {
		t.Fatal("pinned_version membership without pinned_target_version_id was accepted")
	}
}

func createTargetCollectionFixture(t *testing.T, db *DB, ownerID, name, visibility string, accessScopeID *string) *model.TargetCollection {
	t.Helper()
	collection, err := db.CreateTargetCollection(context.Background(), ownerID, model.TargetCollectionCreate{
		Name:           name,
		CollectionKind: "private_map",
		Visibility:     visibility,
		AccessScopeID:  accessScopeID,
		OriginKey:      "manual",
	})
	if err != nil {
		t.Fatalf("CreateTargetCollection %s: %v", name, err)
	}
	return collection
}

func addTargetCollectionMemberFixture(t *testing.T, db *DB, ownerID, collectionID, kind, memberID, role string) {
	t.Helper()
	if _, err := db.AddTargetCollectionMember(context.Background(), ownerID, collectionID, model.TargetCollectionMemberCreate{
		MemberKind: kind,
		MemberID:   &memberID,
		OriginKey:  "manual",
		Role:       role,
	}); err != nil {
		t.Fatalf("AddTargetCollectionMember %s/%s: %v", kind, memberID, err)
	}
}

func assertTargetCollectionIDs(t *testing.T, summaries []model.TargetCollectionSummary, ids ...string) {
	t.Helper()
	for _, id := range ids {
		_ = findTargetCollectionSummary(t, summaries, id)
	}
}

func assertTargetCollectionIDsAbsent(t *testing.T, summaries []model.TargetCollectionSummary, ids ...string) {
	t.Helper()
	for _, id := range ids {
		for _, summary := range summaries {
			if summary.ID == id {
				t.Fatalf("collections unexpectedly contain %s in %#v", id, summaries)
			}
		}
	}
}

func findTargetCollectionSummary(t *testing.T, summaries []model.TargetCollectionSummary, id string) model.TargetCollectionSummary {
	t.Helper()
	for _, summary := range summaries {
		if summary.ID == id {
			return summary
		}
	}
	t.Fatalf("collections missing %s in %#v", id, summaries)
	return model.TargetCollectionSummary{}
}
