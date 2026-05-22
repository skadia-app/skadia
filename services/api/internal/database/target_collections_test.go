package database

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestCollectionCanAddSharedVisibleMember(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collectionOwnerID := insertObjectAccessTestUser(t, db)
	scope := createTargetObjectAccessScope(t, db, ownerID, collectionOwnerID)
	sharedPlace := createTargetPlaceFixture(t, db, ownerID, "shared member", "shared", &scope.ID, `[8,49]`)
	collection := createTargetCollectionFixture(t, db, collectionOwnerID, "collection with shared member", "private", nil)

	member, err := db.AddTargetCollectionMember(ctx, collectionOwnerID, collection.ID, model.TargetCollectionMemberCreate{
		MemberKind: "place",
		MemberID:   &sharedPlace.ID,
		OriginKey:  "manual",
		Role:       "poi",
	})
	if err != nil {
		t.Fatalf("AddTargetCollectionMember shared visible member: %v", err)
	}
	if member.MemberID == nil || *member.MemberID != sharedPlace.ID {
		t.Fatalf("member = %#v, want shared place %s", member, sharedPlace.ID)
	}
}

func TestGovernanceGrantMakesPrivateCollectionReadable(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	managerID := insertObjectAccessTestUser(t, db)
	collection := createTargetCollectionFixture(t, db, ownerID, "private governed collection", "private", nil)
	createTargetObjectGovernanceGrant(t, db, ownerID, managerID, "collection", collection.ID, []string{"manage_members"}, "membership")

	detail, err := db.GetTargetCollection(ctx, managerID, collection.ID, nil, false)
	if err != nil {
		t.Fatalf("GetTargetCollection governed private: %v", err)
	}
	if detail.ID != collection.ID || len(detail.EffectivePermissions) != 1 || detail.EffectivePermissions[0] != "manage_members" {
		t.Fatalf("governed collection detail = %#v", detail.TargetCollection)
	}

	listed, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{UserID: managerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections governed private: %v", err)
	}
	summary := findTargetCollectionSummary(t, listed, collection.ID)
	if len(summary.EffectivePermissions) != 1 || summary.EffectivePermissions[0] != "manage_members" {
		t.Fatalf("governed collection summary = %#v", summary)
	}
}

func TestCollectionDetailMemberVisibilityUsesBatchedQueryShape(t *testing.T) {
	tracer := &targetCollectionQueryCounter{}
	db := targetCollectionTraceDB(t, tracer)
	ctx := context.Background()
	ownerID := insertObjectAccessTestUser(t, db)
	viewerID := insertObjectAccessTestUser(t, db)
	scope := createTargetObjectAccessScope(t, db, ownerID, viewerID)
	collection := createTargetCollectionFixture(t, db, ownerID, "batched detail members", "public", nil)

	publicPlace := createTargetPlaceFixture(t, db, ownerID, "batched public place", "public", nil, `[8,49]`)
	sharedPlace := createTargetPlaceFixture(t, db, ownerID, "batched shared place", "shared", &scope.ID, `[8.001,49.001]`)
	privatePlace := createTargetPlaceFixture(t, db, ownerID, "batched private place", "private", nil, `[8.002,49.002]`)
	governedPlace := createTargetPlaceFixture(t, db, ownerID, "batched governed place", "private", nil, `[8.003,49.003]`)
	createTargetObjectGovernanceGrant(t, db, ownerID, viewerID, "place", governedPlace.ID, []string{"edit_metadata"}, "metadata")

	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", publicPlace.ID, "poi")
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", sharedPlace.ID, "poi")
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", privatePlace.ID, "poi")
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", governedPlace.ID, "poi")

	tracer.Reset()
	detail, err := db.GetTargetCollection(ctx, viewerID, collection.ID, []string{scope.ID}, true)
	if err != nil {
		t.Fatalf("GetTargetCollection viewer: %v", err)
	}
	if got := targetCollectionMemberIDs(detail.Members); !targetCollectionStringSetEqual(got, []string{publicPlace.ID, sharedPlace.ID, governedPlace.ID}) {
		t.Fatalf("viewer members = %v, want public/shared/governed only", got)
	}
	if detail.HiddenMemberCount != 0 {
		t.Fatalf("viewer hidden member count = %d, want 0", detail.HiddenMemberCount)
	}
	if count := tracer.Count(); count > 3 {
		t.Fatalf("GetTargetCollection issued %d queries for 4 members, want constant query shape <= 3", count)
	}

	ownerDetail, err := db.GetTargetCollection(ctx, ownerID, collection.ID, nil, true)
	if err != nil {
		t.Fatalf("GetTargetCollection owner: %v", err)
	}
	if got := targetCollectionMemberIDs(ownerDetail.Members); !targetCollectionStringSetEqual(got, []string{publicPlace.ID, sharedPlace.ID, privatePlace.ID, governedPlace.ID}) {
		t.Fatalf("owner members = %v, want all members", got)
	}
	if ownerDetail.HiddenMemberCount != 3 {
		t.Fatalf("owner hidden member count = %d, want 3", ownerDetail.HiddenMemberCount)
	}
}

func TestCollectionSummaryDoesNotLeakPrivateMemberEnvelopeOrCounts(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collection := createTargetCollectionFixture(t, db, ownerID, "public private-only", "public", nil)
	privatePlace := createTargetPlaceFixture(t, db, ownerID, "private only member", "private", nil, `[8,49]`)
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", privatePlace.ID, "poi")

	publicSummaries, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections public: %v", err)
	}
	publicSummary := findTargetCollectionSummary(t, publicSummaries, collection.ID)
	if publicSummary.BoundsGeoJSON != nil {
		t.Fatalf("public summary leaked private member bounds: %s", *publicSummary.BoundsGeoJSON)
	}
	if publicSummary.OwnerMemberCount != 0 || publicSummary.PublicMemberCount != 0 {
		t.Fatalf("public counts owner=%d public=%d, want 0/0", publicSummary.OwnerMemberCount, publicSummary.PublicMemberCount)
	}

	ownerSummaries, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{UserID: ownerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections owner: %v", err)
	}
	ownerSummary := findTargetCollectionSummary(t, ownerSummaries, collection.ID)
	if ownerSummary.OwnerMemberCount != 1 {
		t.Fatalf("owner count = %d, want 1", ownerSummary.OwnerMemberCount)
	}
}

func TestTargetCollectionExternalRefsAreViewerRedacted(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	refKind := "source"
	refID := uuid.NewString()
	refValue := "source:collection-private"
	collection, err := db.CreateTargetCollection(ctx, ownerID, model.TargetCollectionCreate{
		Name:                  "redacted collection",
		CollectionKind:        "private_map",
		Visibility:            "public",
		OriginKey:             "manual",
		ExternalRefKind:       &refKind,
		ExternalRefID:         &refID,
		ExternalRef:           &refValue,
		ExternalRefVisibility: "owner_only",
	})
	if err != nil {
		t.Fatalf("CreateTargetCollection: %v", err)
	}

	publicDetail, err := db.GetTargetCollection(ctx, "", collection.ID, nil, false)
	if err != nil {
		t.Fatalf("GetTargetCollection public: %v", err)
	}
	if publicDetail.ExternalRefKind != nil || publicDetail.ExternalRefID != nil || publicDetail.ExternalRef != nil {
		t.Fatalf("public collection leaked external refs: %#v", publicDetail.TargetCollection)
	}

	ownerDetail, err := db.GetTargetCollection(ctx, ownerID, collection.ID, nil, false)
	if err != nil {
		t.Fatalf("GetTargetCollection owner: %v", err)
	}
	if ownerDetail.ExternalRefKind == nil || ownerDetail.ExternalRefID == nil || ownerDetail.ExternalRef == nil {
		t.Fatalf("owner collection refs were redacted: %#v", ownerDetail.TargetCollection)
	}
}

func TestCollectionRejectsUnknownCourseMember(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collection := createTargetCollectionFixture(t, db, ownerID, "unknown course", "private", nil)
	missingID := uuid.NewString()
	_, err := db.AddTargetCollectionMember(ctx, ownerID, collection.ID, model.TargetCollectionMemberCreate{
		MemberKind: "course",
		MemberID:   &missingID,
		OriginKey:  "manual",
	})
	if err == nil {
		t.Fatal("collection accepted unknown course member")
	}
}

func TestGovernanceManageMembersCanEditCollectionMembers(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	managerID := insertObjectAccessTestUser(t, db)
	collection := createTargetCollectionFixture(t, db, ownerID, "delegated collection", "public", nil)
	place := createTargetPlaceFixture(t, db, ownerID, "delegated member", "public", nil, `[8,49]`)
	privatePlace := createTargetPlaceFixture(t, db, ownerID, "delegated private member", "private", nil, `[8.01,49.01]`)
	scope, err := db.CreateGovernanceScope(ctx, ownerID, model.GovernanceScopeCreate{
		Kind:               "collection_scope",
		AnchorCollectionID: &collection.ID,
		OwnerScope:         "user",
		OriginKey:          "manual",
	})
	if err != nil {
		t.Fatalf("CreateGovernanceScope: %v", err)
	}
	addGovernanceRoleFixture(t, db, ownerID, scope.ID, managerID, []string{"manage_members"})
	addGovernanceBindingFixture(t, db, ownerID, scope.ID, "collection", collection.ID, "membership")

	if _, err := db.AddTargetCollectionMember(ctx, managerID, collection.ID, model.TargetCollectionMemberCreate{
		MemberKind: "place",
		MemberID:   &place.ID,
		OriginKey:  "manual",
		Role:       "poi",
	}); err != nil {
		t.Fatalf("delegated AddTargetCollectionMember: %v", err)
	}
	if _, err := db.AddTargetCollectionMember(ctx, ownerID, collection.ID, model.TargetCollectionMemberCreate{
		MemberKind:           "place",
		MemberID:             &privatePlace.ID,
		MembershipVisibility: "owner_only",
		OriginKey:            "manual",
		Role:                 "poi",
	}); err != nil {
		t.Fatalf("owner AddTargetCollectionMember private: %v", err)
	}
	detail, err := db.GetTargetCollection(ctx, managerID, collection.ID, nil, true)
	if err != nil {
		t.Fatalf("manager GetTargetCollection: %v", err)
	}
	if got := targetCollectionMemberIDs(detail.Members); !targetCollectionStringSetEqual(got, []string{place.ID, privatePlace.ID}) {
		t.Fatalf("manager members = %v, want public and owner_only members", got)
	}
	if detail.HiddenMemberCount == 0 {
		t.Fatal("manager detail did not expose hidden member count")
	}
}

func TestCollectionSummaryProjectionTracksBoundsAndCounts(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	collection := createTargetCollectionFixture(t, db, ownerID, "bounds", "public", nil)
	place := createTargetPlaceFixture(t, db, ownerID, "bounds place", "public", nil, `[8,49]`)
	region := createTargetRegionFixture(t, db, ownerID, "bounds region", "public", nil, `[[[8.01,49.01],[8.02,49.01],[8.02,49.02],[8.01,49.02],[8.01,49.01]]]`)
	route, snapshot := createRoutePublicationTestSnapshot(t, db, ownerID, "bounds route publication")
	if _, err := db.CreateRoutePublicationPrivacyReview(ctx, route.ID, snapshot.ID, ownerID, snapshot.SourceRouteVersion, "public", true); err != nil {
		t.Fatalf("CreateRoutePublicationPrivacyReview: %v", err)
	}
	publication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         ownerID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "public",
		CompatibilityPreset: "standard",
		OperationID:         "collection-summary-publication-" + collection.ID,
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse: %v", err)
	}

	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "place", place.ID, "poi")
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "region", region.ID, "area")
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "course", publication.CourseSegmentID, "trail")
	addTargetCollectionMemberFixture(t, db, ownerID, collection.ID, "route_publication", publication.ID, "variant")

	summaries, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{UserID: ownerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetCollections: %v", err)
	}
	summary := findTargetCollectionSummary(t, summaries, collection.ID)
	if summary.OwnerMemberCount != 4 || summary.PublicMemberCount != 4 {
		t.Fatalf("summary counts owner=%d public=%d, want 4/4", summary.OwnerMemberCount, summary.PublicMemberCount)
	}
	assertCollectionCountMap(t, summary.OwnerCountsByKind, map[string]int{"place": 1, "region": 1, "course": 1, "route_publication": 1})
	assertCollectionCountMap(t, summary.PublicCountsByKind, map[string]int{"place": 1, "region": 1, "course": 1, "route_publication": 1})
	assertCollectionCountMap(t, summary.OwnerCountsByRole, map[string]int{"poi": 1, "area": 1, "trail": 1, "variant": 1})
	assertCollectionCountMap(t, summary.PublicCountsByRole, map[string]int{"poi": 1, "area": 1, "trail": 1, "variant": 1})
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
	summaries, err := db.ListTargetCollections(ctx, model.ListTargetCollectionsParams{UserID: ownerID, Limit: 20})
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

type targetCollectionQueryCounter struct {
	enabled atomic.Bool
	count   atomic.Int64
}

func (c *targetCollectionQueryCounter) Reset() {
	c.count.Store(0)
	c.enabled.Store(true)
}

func (c *targetCollectionQueryCounter) Count() int64 {
	c.enabled.Store(false)
	return c.count.Load()
}

func (c *targetCollectionQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if c.enabled.Load() && strings.TrimSpace(data.SQL) != "" {
		c.count.Add(1)
	}
	return ctx
}

func (c *targetCollectionQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {
}

func targetCollectionTraceDB(t *testing.T, tracer pgx.QueryTracer) *DB {
	t.Helper()
	config, err := pgxpool.ParseConfig(testdb.RequireSafeDSN(t))
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	config.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	return &DB{Pool: pool}
}

func targetCollectionMemberIDs(members []model.TargetCollectionMember) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		if member.MemberID != nil {
			ids = append(ids, *member.MemberID)
		}
	}
	return ids
}

func targetCollectionStringSetEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(got))
	for _, value := range got {
		counts[value]++
	}
	for _, value := range want {
		if counts[value] == 0 {
			return false
		}
		counts[value]--
	}
	return true
}

func assertCollectionCountMap(t *testing.T, raw json.RawMessage, want map[string]int) {
	t.Helper()
	var got map[string]int
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal collection counts %s: %v", string(raw), err)
	}
	if len(got) != len(want) {
		t.Fatalf("collection counts = %#v, want %#v", got, want)
	}
	for key, wantCount := range want {
		if got[key] != wantCount {
			t.Fatalf("collection count[%s] = %d, want %d in %#v", key, got[key], wantCount, got)
		}
	}
}
