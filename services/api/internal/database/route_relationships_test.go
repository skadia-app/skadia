package database

import (
	"context"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestForkRoute(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	source := createRouteRelationshipTestRoute(t, db, userID, "source route")

	fork, relationshipID, err := db.ForkRoute(ctx, source.ID, userID, "forked route", "fork-op-1", "hash:fork-1", source.Version)
	if err != nil {
		t.Fatalf("ForkRoute: %v", err)
	}
	if fork.ID == source.ID || relationshipID == "" {
		t.Fatalf("fork failed: route=%#v relationship=%q", fork, relationshipID)
	}
	if fork.Name != "forked route" || fork.GeometryGeoJSON != source.GeometryGeoJSON || len(fork.Waypoints) != len(source.Waypoints) || len(fork.Segments) != len(source.Segments) {
		t.Fatalf("fork did not copy route state: fork=%#v source=%#v", fork, source)
	}
	var photoCount int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM route_photos WHERE route_id = $1`, fork.ID).Scan(&photoCount); err != nil {
		t.Fatalf("count fork photos: %v", err)
	}
	if photoCount != 0 {
		t.Fatalf("fork copied photos: %d", photoCount)
	}

	retry, retryRelationshipID, err := db.ForkRoute(ctx, source.ID, userID, "forked route", "fork-op-1", "hash:fork-1", source.Version)
	if err != nil {
		t.Fatalf("ForkRoute retry: %v", err)
	}
	if retry.ID != fork.ID || retryRelationshipID != relationshipID {
		t.Fatalf("idempotent fork = %s/%s, want %s/%s", retry.ID, retryRelationshipID, fork.ID, relationshipID)
	}

	_, _, err = db.ForkRoute(ctx, source.ID, userID, "different fork", "fork-op-1", "hash:different", source.Version)
	if !errors.Is(err, ErrRoutePublicationConflict) {
		t.Fatalf("conflicting fork op error = %v, want ErrRoutePublicationConflict", err)
	}
}

func TestRouteVariantRelationship(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	source := createRouteRelationshipTestRoute(t, db, userID, "variant source")
	target := createRouteRelationshipTestRoute(t, db, userID, "variant target")
	sourceGeometry := source.GeometryGeoJSON
	targetGeometry := target.GeometryGeoJSON

	relationshipID, err := db.CreateRouteRelationship(ctx, source.ID, target.ID, userID, "variant_of", "variant-op-1", "hash:variant-1")
	if err != nil {
		t.Fatalf("CreateRouteRelationship: %v", err)
	}
	if relationshipID == "" {
		t.Fatal("CreateRouteRelationship returned empty id")
	}

	reloadedSource, err := db.GetRoute(ctx, source.ID, userID)
	if err != nil {
		t.Fatalf("GetRoute source: %v", err)
	}
	reloadedTarget, err := db.GetRoute(ctx, target.ID, userID)
	if err != nil {
		t.Fatalf("GetRoute target: %v", err)
	}
	if reloadedSource.GeometryGeoJSON != sourceGeometry || reloadedTarget.GeometryGeoJSON != targetGeometry {
		t.Fatal("variant relation changed route geometry")
	}

	retryID, err := db.CreateRouteRelationship(ctx, source.ID, target.ID, userID, "variant_of", "variant-op-1", "hash:variant-1")
	if err != nil {
		t.Fatalf("CreateRouteRelationship retry: %v", err)
	}
	if retryID != relationshipID {
		t.Fatalf("idempotent relationship id = %s, want %s", retryID, relationshipID)
	}
}

func TestRouteRelationshipIdempotencyRejectsDifferentSource(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	source := createRouteRelationshipTestRoute(t, db, userID, "variant source a")
	otherSource := createRouteRelationshipTestRoute(t, db, userID, "variant source b")
	target := createRouteRelationshipTestRoute(t, db, userID, "variant target")

	relationshipID, err := db.CreateRouteRelationship(ctx, source.ID, target.ID, userID, "variant_of", "variant-op-reused-source", "hash:variant-reused-source")
	if err != nil {
		t.Fatalf("CreateRouteRelationship first: %v", err)
	}
	if relationshipID == "" {
		t.Fatal("CreateRouteRelationship first returned empty id")
	}

	_, err = db.CreateRouteRelationship(ctx, otherSource.ID, target.ID, userID, "variant_of", "variant-op-reused-source", "hash:variant-reused-source")
	if !errors.Is(err, ErrRoutePublicationConflict) {
		t.Fatalf("reused operation for different source error = %v, want ErrRoutePublicationConflict", err)
	}
}

func TestForkRouteIdempotencyRejectsDifferentSource(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	source := createRouteRelationshipTestRoute(t, db, userID, "fork source a")
	otherSource := createRouteRelationshipTestRoute(t, db, userID, "fork source b")

	fork, relationshipID, err := db.ForkRoute(ctx, source.ID, userID, "forked route", "fork-op-reused-source", "hash:fork-reused-source", source.Version)
	if err != nil {
		t.Fatalf("ForkRoute first: %v", err)
	}
	if fork.ID == "" || relationshipID == "" {
		t.Fatalf("ForkRoute first missing ids: route=%#v relationship=%q", fork, relationshipID)
	}

	_, _, err = db.ForkRoute(ctx, otherSource.ID, userID, "forked route", "fork-op-reused-source", "hash:fork-reused-source", otherSource.Version)
	if !errors.Is(err, ErrRoutePublicationConflict) {
		t.Fatalf("reused fork operation for different source error = %v, want ErrRoutePublicationConflict", err)
	}
}

func TestRouteRelationshipOwnership(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	otherUserID := insertObjectAccessTestUser(t, db)
	source := createRouteRelationshipTestRoute(t, db, userID, "owned source")
	other := createRouteRelationshipTestRoute(t, db, otherUserID, "other target")
	target := createRouteRelationshipTestRoute(t, db, userID, "owned target")

	if _, err := db.CreateRouteRelationship(ctx, source.ID, source.ID, userID, "variant_of", "self-op", "hash:self"); !errors.Is(err, ErrRoutePublicationValidation) {
		t.Fatalf("self relation error = %v, want ErrRoutePublicationValidation", err)
	}
	if _, err := db.CreateRouteRelationship(ctx, source.ID, other.ID, userID, "variant_of", "foreign-op", "hash:foreign"); !errors.Is(err, ErrRoutePublicationValidation) {
		t.Fatalf("foreign relation error = %v, want ErrRoutePublicationValidation", err)
	}
	if _, err := db.CreateRouteRelationship(ctx, source.ID, target.ID, userID, "fork_of", "manual-fork-op", "hash:manual-fork"); !errors.Is(err, ErrRoutePublicationValidation) {
		t.Fatalf("manual fork_of relation error = %v, want ErrRoutePublicationValidation", err)
	}
}

func TestRouteCourseVariantRelationship(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, firstSnapshot := createRouteRelationshipPublicationSnapshot(t, db, userID, "first publication")
	_, secondSnapshot := createRouteRelationshipPublicationSnapshot(t, db, userID, "second publication")
	firstPublication := publishRouteRelationshipTestCourse(t, db, userID, firstSnapshot, "course-rel-publish-1")
	secondPublication := publishRouteRelationshipTestCourse(t, db, userID, secondSnapshot, "course-rel-publish-2")

	relationshipID, err := db.CreateRouteCourseRelationship(ctx, firstPublication.ID, secondPublication.ID, userID, "variant_of", "course-rel-op-1", "hash:course-rel-1")
	if err != nil {
		t.Fatalf("CreateRouteCourseRelationship: %v", err)
	}
	if relationshipID == "" {
		t.Fatal("CreateRouteCourseRelationship returned empty id")
	}
	var sourceVersion, targetVersion string
	if err := db.Pool.QueryRow(ctx, `
		SELECT source_course_segment_version_id, target_course_segment_version_id
		FROM route_course_relationships
		WHERE id = $1
	`, relationshipID).Scan(&sourceVersion, &targetVersion); err != nil {
		t.Fatalf("load course relationship: %v", err)
	}
	if sourceVersion != firstPublication.CourseSegmentVersionID || targetVersion != secondPublication.CourseSegmentVersionID {
		t.Fatalf("course relation versions = %s/%s, want %s/%s", sourceVersion, targetVersion, firstPublication.CourseSegmentVersionID, secondPublication.CourseSegmentVersionID)
	}
}

func TestRouteCourseRelationshipIdempotencyRejectsDifferentSourcePublication(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, firstSnapshot := createRouteRelationshipPublicationSnapshot(t, db, userID, "course source a")
	_, otherSourceSnapshot := createRouteRelationshipPublicationSnapshot(t, db, userID, "course source b")
	_, targetSnapshot := createRouteRelationshipPublicationSnapshot(t, db, userID, "course target")
	firstPublication := publishRouteRelationshipTestCourse(t, db, userID, firstSnapshot, "course-rel-reused-source-publish-1")
	otherSourcePublication := publishRouteRelationshipTestCourse(t, db, userID, otherSourceSnapshot, "course-rel-reused-source-publish-2")
	targetPublication := publishRouteRelationshipTestCourse(t, db, userID, targetSnapshot, "course-rel-reused-source-publish-3")

	relationshipID, err := db.CreateRouteCourseRelationship(ctx, firstPublication.ID, targetPublication.ID, userID, "variant_of", "course-rel-op-reused-source", "hash:course-rel-reused-source")
	if err != nil {
		t.Fatalf("CreateRouteCourseRelationship first: %v", err)
	}
	if relationshipID == "" {
		t.Fatal("CreateRouteCourseRelationship first returned empty id")
	}

	_, err = db.CreateRouteCourseRelationship(ctx, otherSourcePublication.ID, targetPublication.ID, userID, "variant_of", "course-rel-op-reused-source", "hash:course-rel-reused-source")
	if !errors.Is(err, ErrRoutePublicationConflict) {
		t.Fatalf("reused course operation for different source error = %v, want ErrRoutePublicationConflict", err)
	}
}

func createRouteRelationshipTestRoute(t *testing.T, db *DB, userID, name string) *model.PlannedRoute {
	t.Helper()
	subType := "mountain"
	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            name,
		SportType:       "cycling",
		SubType:         &subType,
		Profile:         "cycling-mountain",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[7.000,47.000],[7.001,47.001]]}`,
		Waypoints:       []model.RouteWaypoint{{ID: "a", Type: "start", LatLng: [2]float64{47.000, 7.000}}, {ID: "b", Type: "end", LatLng: [2]float64{47.001, 7.001}}},
		Segments:        []model.RouteSegment{{FromWaypoint: "a", ToWaypoint: "b", Mode: "routed", DistanceM: 100, Geometry: &model.GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{7.000, 47.000}, {7.001, 47.001}}}}},
	}
	id, err := db.CreateRoute(context.Background(), route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	loaded, err := db.GetRoute(context.Background(), id, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	return loaded
}

func createRouteRelationshipPublicationSnapshot(t *testing.T, db *DB, userID, name string) (*model.PlannedRoute, *model.PlannedRouteSnapshot) {
	t.Helper()
	route := createRouteRelationshipTestRoute(t, db, userID, name)
	snapshot, err := db.CreatePlannedRouteSnapshot(context.Background(), model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             userID,
		SourceRouteVersion: route.Version,
		OperationID:        "relationship-snapshot-" + route.ID,
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot: %v", err)
	}
	return route, snapshot
}

func publishRouteRelationshipTestCourse(t *testing.T, db *DB, userID string, snapshot *model.PlannedRouteSnapshot, operationID string) *model.RouteCoursePublication {
	t.Helper()
	publication, err := db.PublishRouteSnapshotAsCourse(context.Background(), model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         operationID,
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse: %v", err)
	}
	return publication
}
