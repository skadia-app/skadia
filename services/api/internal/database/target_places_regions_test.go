package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/google/uuid"
)

func TestCreateListTargetPlaceWithVisibility(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	scope := createTargetObjectAccessScope(t, db, ownerID, viewerID)

	privatePlace := createTargetPlaceFixture(t, db, ownerID, "private trailhead", "private", nil, `[8,49]`)
	publicPlace := createTargetPlaceFixture(t, db, ownerID, "public trailhead", "public", nil, `[8.001,49.001]`)
	sharedPlace := createTargetPlaceFixture(t, db, ownerID, "shared trailhead", "shared", &scope.ID, `[8.002,49.002]`)

	ownerPlaces, err := db.ListTargetPlaces(ctx, model.ListTargetObjectsParams{UserID: ownerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetPlaces owner: %v", err)
	}
	assertTargetPlaceIDs(t, ownerPlaces, privatePlace.ID, publicPlace.ID, sharedPlace.ID)

	publicPlaces, err := db.ListTargetPlaces(ctx, model.ListTargetObjectsParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetPlaces public: %v", err)
	}
	assertTargetPlaceIDs(t, publicPlaces, publicPlace.ID)
	assertTargetPlaceIDsAbsent(t, publicPlaces, privatePlace.ID, sharedPlace.ID)

	resolvedScopes, err := db.ResolveVisibleAccessScopes(ctx, viewerID, nil, nil)
	if err != nil {
		t.Fatalf("ResolveVisibleAccessScopes: %v", err)
	}
	viewerPlaces, err := db.ListTargetPlaces(ctx, model.ListTargetObjectsParams{
		UserID:                viewerID,
		VisibleAccessScopeIDs: resolvedScopes,
		Limit:                 20,
	})
	if err != nil {
		t.Fatalf("ListTargetPlaces viewer: %v", err)
	}
	assertTargetPlaceIDs(t, viewerPlaces, publicPlace.ID, sharedPlace.ID)
	assertTargetPlaceIDsAbsent(t, viewerPlaces, privatePlace.ID)
}

func TestCreateListTargetRegionWithBounds(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()

	inside := createTargetRegionFixture(t, db, ownerID, "inside", "public", nil, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)
	outside := createTargetRegionFixture(t, db, ownerID, "outside", "public", nil, `[[[9,50],[9.01,50],[9.01,50.01],[9,50.01],[9,50]]]`)

	regions, err := db.ListTargetRegions(ctx, model.ListTargetObjectsParams{
		BBox:  &model.GeoBBox{MinLon: 7.99, MinLat: 48.99, MaxLon: 8.02, MaxLat: 49.02},
		Limit: 20,
	})
	if err != nil {
		t.Fatalf("ListTargetRegions bbox: %v", err)
	}
	assertTargetRegionIDs(t, regions, inside.ID)
	assertTargetRegionIDsAbsent(t, regions, outside.ID)
}

func TestSharedTargetObjectsRequireResolvedScope(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	scope := createTargetObjectAccessScope(t, db, ownerID, viewerID)
	sharedPlace := createTargetPlaceFixture(t, db, ownerID, "shared only", "shared", &scope.ID, `[8,49]`)

	withoutScope, err := db.ListTargetPlaces(ctx, model.ListTargetObjectsParams{UserID: viewerID, Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetPlaces without scope: %v", err)
	}
	assertTargetPlaceIDsAbsent(t, withoutScope, sharedPlace.ID)

	withScope, err := db.ListTargetPlaces(ctx, model.ListTargetObjectsParams{
		UserID:                viewerID,
		VisibleAccessScopeIDs: []string{scope.ID},
		Limit:                 20,
	})
	if err != nil {
		t.Fatalf("ListTargetPlaces with scope: %v", err)
	}
	assertTargetPlaceIDs(t, withScope, sharedPlace.ID)
}

func TestTargetObjectExternalRefsAreViewerRedacted(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	refKind := "source"
	refID := uuid.NewString()
	refValue := "source:private"
	place, err := db.CreateTargetPlace(ctx, ownerID, model.TargetPlaceCreate{
		Name:                  "redacted place",
		PlaceKind:             "custom",
		GeometryGeoJSON:       `{"type":"Point","coordinates":[8,49]}`,
		Visibility:            "public",
		OriginKey:             "manual",
		ExternalRefKind:       &refKind,
		ExternalRefID:         &refID,
		ExternalRef:           &refValue,
		ExternalRefVisibility: "owner_only",
	})
	if err != nil {
		t.Fatalf("CreateTargetPlace: %v", err)
	}

	publicPlace, err := db.GetTargetPlace(ctx, "", place.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetPlace public: %v", err)
	}
	if publicPlace.ExternalRefKind != nil || publicPlace.ExternalRefID != nil || publicPlace.ExternalRef != nil {
		t.Fatalf("public place leaked external refs: %#v", publicPlace)
	}

	ownerPlace, err := db.GetTargetPlace(ctx, ownerID, place.ID, nil)
	if err != nil {
		t.Fatalf("GetTargetPlace owner: %v", err)
	}
	if ownerPlace.ExternalRefKind == nil || ownerPlace.ExternalRefID == nil || ownerPlace.ExternalRef == nil {
		t.Fatalf("owner place refs were redacted: %#v", ownerPlace)
	}
}

func TestArchiveTargetPlaceAndRegionRemovesFromProjection(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	place := createTargetPlaceFixture(t, db, ownerID, "archived place", "public", nil, `[8,49]`)
	region := createTargetRegionFixture(t, db, ownerID, "archived region", "public", nil, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)

	if err := db.ArchiveTargetPlace(ctx, ownerID, place.ID); err != nil {
		t.Fatalf("ArchiveTargetPlace: %v", err)
	}
	if err := db.ArchiveTargetRegion(ctx, ownerID, region.ID); err != nil {
		t.Fatalf("ArchiveTargetRegion: %v", err)
	}

	places, err := db.ListTargetPlaces(ctx, model.ListTargetObjectsParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetPlaces after archive: %v", err)
	}
	regions, err := db.ListTargetRegions(ctx, model.ListTargetObjectsParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListTargetRegions after archive: %v", err)
	}
	assertTargetPlaceIDsAbsent(t, places, place.ID)
	assertTargetRegionIDsAbsent(t, regions, region.ID)

	var archivedPlace, archivedRegion bool
	if err := db.Pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM target_places WHERE id = $1`, place.ID).Scan(&archivedPlace); err != nil {
		t.Fatalf("query archived place: %v", err)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM target_regions WHERE id = $1`, region.ID).Scan(&archivedRegion); err != nil {
		t.Fatalf("query archived region: %v", err)
	}
	if !archivedPlace || !archivedRegion {
		t.Fatalf("archive state place=%v region=%v, want both true", archivedPlace, archivedRegion)
	}
}

func TestRevokingAccessScopeDemotesTargetObjects(t *testing.T) {
	db, ownerID := segmentPRTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	scope := createTargetObjectAccessScope(t, db, ownerID, viewerID)
	place := createTargetPlaceFixture(t, db, ownerID, "shared place", "shared", &scope.ID, `[8,49]`)
	region := createTargetRegionFixture(t, db, ownerID, "shared region", "shared", &scope.ID, `[[[8,49],[8.01,49],[8.01,49.01],[8,49.01],[8,49]]]`)
	collectionID := insertSharedTargetCollectionFixture(t, db, ownerID, scope.ID)

	if err := db.RevokeObjectAccessScope(ctx, scope.ID, ownerID); err != nil {
		t.Fatalf("RevokeObjectAccessScope: %v", err)
	}

	assertTargetAccessState(t, db, "target_places", place.ID, "private", false)
	assertTargetAccessState(t, db, "target_regions", region.ID, "private", false)
	assertTargetAccessState(t, db, "target_collections", collectionID, "private", false)
	assertTargetCollectionProjectionAccessState(t, db, collectionID, "private", false)
}

func createTargetObjectAccessScope(t *testing.T, db *DB, ownerID, viewerID string) *model.ObjectAccessScope {
	t.Helper()
	scope, err := db.CreateObjectAccessScope(context.Background(), model.ObjectAccessScopeInput{
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
	return scope
}

func createTargetPlaceFixture(t *testing.T, db *DB, ownerID, name, visibility string, accessScopeID *string, pointCoordinates string) *model.TargetPlace {
	t.Helper()
	place, err := db.CreateTargetPlace(context.Background(), ownerID, model.TargetPlaceCreate{
		Name:            name,
		PlaceKind:       "custom",
		GeometryGeoJSON: `{"type":"Point","coordinates":` + pointCoordinates + `}`,
		Visibility:      visibility,
		AccessScopeID:   accessScopeID,
		OriginKey:       "manual",
	})
	if err != nil {
		t.Fatalf("CreateTargetPlace %s: %v", name, err)
	}
	return place
}

func createTargetRegionFixture(t *testing.T, db *DB, ownerID, name, visibility string, accessScopeID *string, polygonCoordinates string) *model.TargetRegion {
	t.Helper()
	region, err := db.CreateTargetRegion(context.Background(), ownerID, model.TargetRegionCreate{
		Name:            name,
		RegionKind:      "custom",
		GeometryGeoJSON: `{"type":"Polygon","coordinates":` + polygonCoordinates + `}`,
		Visibility:      visibility,
		AccessScopeID:   accessScopeID,
		OriginKey:       "manual",
	})
	if err != nil {
		t.Fatalf("CreateTargetRegion %s: %v", name, err)
	}
	return region
}

func insertSharedTargetCollectionFixture(t *testing.T, db *DB, ownerID, scopeID string) string {
	t.Helper()
	collectionID := uuid.NewString()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO target_collections (
			id, owner_user_id, name, collection_kind, visibility, access_scope_id, origin_key, created_by
		)
		VALUES ($1, $2, 'shared collection', 'private_map', 'shared', $3, 'manual', $2)
	`, collectionID, ownerID, scopeID); err != nil {
		t.Fatalf("insert shared collection: %v", err)
	}
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO target_collection_summary_projection (
			collection_id, owner_user_id, lifecycle_owner_kind, name, collection_kind,
			visibility, access_scope_id, updated_at
		)
		VALUES ($1, $2, 'user', 'shared collection', 'private_map', 'shared', $3, NOW())
	`, collectionID, ownerID, scopeID); err != nil {
		t.Fatalf("insert shared collection projection: %v", err)
	}
	return collectionID
}

func assertTargetPlaceIDs(t *testing.T, places []model.TargetPlace, ids ...string) {
	t.Helper()
	for _, id := range ids {
		found := false
		for _, place := range places {
			if place.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("places missing %s in %#v", id, places)
		}
	}
}

func assertTargetPlaceIDsAbsent(t *testing.T, places []model.TargetPlace, ids ...string) {
	t.Helper()
	for _, id := range ids {
		for _, place := range places {
			if place.ID == id {
				t.Fatalf("places unexpectedly contain %s in %#v", id, places)
			}
		}
	}
}

func assertTargetRegionIDs(t *testing.T, regions []model.TargetRegion, ids ...string) {
	t.Helper()
	for _, id := range ids {
		found := false
		for _, region := range regions {
			if region.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("regions missing %s in %#v", id, regions)
		}
	}
}

func assertTargetRegionIDsAbsent(t *testing.T, regions []model.TargetRegion, ids ...string) {
	t.Helper()
	for _, id := range ids {
		for _, region := range regions {
			if region.ID == id {
				t.Fatalf("regions unexpectedly contain %s in %#v", id, regions)
			}
		}
	}
}

func assertTargetAccessState(t *testing.T, db *DB, table, id, wantVisibility string, wantScope bool) {
	t.Helper()
	var visibility string
	var hasScope bool
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT visibility, access_scope_id IS NOT NULL
		FROM `+table+`
		WHERE id = $1
	`, id).Scan(&visibility, &hasScope); err != nil {
		t.Fatalf("query %s access state: %v", table, err)
	}
	if visibility != wantVisibility || hasScope != wantScope {
		t.Fatalf("%s access state = %s/%v, want %s/%v", table, visibility, hasScope, wantVisibility, wantScope)
	}
}

func assertTargetCollectionProjectionAccessState(t *testing.T, db *DB, collectionID, wantVisibility string, wantScope bool) {
	t.Helper()
	var visibility string
	var hasScope bool
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT visibility, access_scope_id IS NOT NULL
		FROM target_collection_summary_projection
		WHERE collection_id = $1
	`, collectionID).Scan(&visibility, &hasScope); err != nil {
		t.Fatalf("query collection projection access state: %v", err)
	}
	if visibility != wantVisibility || hasScope != wantScope {
		t.Fatalf("collection projection access state = %s/%v, want %s/%v", visibility, hasScope, wantVisibility, wantScope)
	}
}
