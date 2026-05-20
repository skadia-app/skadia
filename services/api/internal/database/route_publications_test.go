package database

import (
	"context"
	"errors"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestPublishRouteSnapshotAsCourse(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	route, snapshot := createRoutePublicationTestSnapshot(t, db, userID, "published route")

	publication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-op-1",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse: %v", err)
	}
	if publication.ID == "" || publication.CourseSegmentID == "" || publication.CourseSegmentVersionID == "" {
		t.Fatalf("publication missing ids: %#v", publication)
	}

	loadedRoute, err := db.GetRoute(ctx, route.ID, userID)
	if err != nil {
		t.Fatalf("GetRoute after publish: %v", err)
	}
	if loadedRoute.Version != route.Version {
		t.Fatalf("route version changed by publish: got %d want %d", loadedRoute.Version, route.Version)
	}

	var versionCount, profileCount, projectionCount int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM segment_versions WHERE segment_id = $1`, publication.CourseSegmentID).Scan(&versionCount); err != nil {
		t.Fatalf("count segment_versions: %v", err)
	}
	if versionCount != 1 {
		t.Fatalf("segment_versions count = %d, want 1", versionCount)
	}
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM route_profile_snapshots WHERE route_snapshot_id = $1`, snapshot.ID).Scan(&profileCount); err != nil {
		t.Fatalf("count route_profile_snapshots: %v", err)
	}
	if profileCount != 1 {
		t.Fatalf("route_profile_snapshots count = %d, want 1", profileCount)
	}
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM route_course_profile_projection
		WHERE subject_kind = 'published_course'
		  AND subject_id = $1
		  AND subject_version_id = $2
	`, publication.CourseSegmentID, publication.CourseSegmentVersionID).Scan(&projectionCount); err != nil {
		t.Fatalf("count course projection: %v", err)
	}
	if projectionCount != 1 {
		t.Fatalf("course projection count = %d, want 1", projectionCount)
	}
}

func TestRoutePublicationIdempotency(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, firstSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "first route")
	_, secondSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "second route")

	input := model.PublishRouteSnapshotInput{
		RouteSnapshotID:     firstSnapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  firstSnapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-idempotent",
	}
	first, err := db.PublishRouteSnapshotAsCourse(ctx, input)
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse first: %v", err)
	}
	second, err := db.PublishRouteSnapshotAsCourse(ctx, input)
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse retry: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotent retry returned %s, want %s", second.ID, first.ID)
	}

	input.RouteSnapshotID = secondSnapshot.ID
	input.SourceRouteVersion = secondSnapshot.SourceRouteVersion
	_, err = db.PublishRouteSnapshotAsCourse(ctx, input)
	if !errors.Is(err, ErrRoutePublicationConflict) {
		t.Fatalf("conflicting operation error = %v, want ErrRoutePublicationConflict", err)
	}
}

func TestRoutePublicationFreezesGeometry(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	route, snapshot := createRoutePublicationTestSnapshot(t, db, userID, "frozen route")

	publication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-freeze",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse: %v", err)
	}
	var beforeGeometry string
	if err := db.Pool.QueryRow(ctx, `
		SELECT ST_AsGeoJSON(track)
		FROM segment_versions
		WHERE id = $1
	`, publication.CourseSegmentVersionID).Scan(&beforeGeometry); err != nil {
		t.Fatalf("load published geometry: %v", err)
	}

	route.GeometryGeoJSON = `{"type":"LineString","coordinates":[[8.000,48.000],[8.010,48.010]]}`
	route.Name = "edited after publish"
	if err := db.UpdateRoute(ctx, route, route.Version); err != nil {
		t.Fatalf("UpdateRoute after publish: %v", err)
	}
	if err := db.DeleteRoute(ctx, route.ID, userID); err != nil {
		t.Fatalf("DeleteRoute after publish: %v", err)
	}

	var afterGeometry string
	if err := db.Pool.QueryRow(ctx, `
		SELECT ST_AsGeoJSON(track)
		FROM segment_versions
		WHERE id = $1
	`, publication.CourseSegmentVersionID).Scan(&afterGeometry); err != nil {
		t.Fatalf("reload published geometry: %v", err)
	}
	if afterGeometry != beforeGeometry {
		t.Fatalf("published geometry changed:\nbefore=%s\nafter=%s", beforeGeometry, afterGeometry)
	}
	var publicationExists bool
	if err := db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM route_course_publications WHERE id = $1)`, publication.ID).Scan(&publicationExists); err != nil {
		t.Fatalf("check publication exists: %v", err)
	}
	if !publicationExists {
		t.Fatal("publication was deleted with source route")
	}
}

func TestRoutePublicationDoesNotCreateGuideContent(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	description := "## Tour notes\n\nThis is route metadata, not a guide article."
	_, snapshot := createRoutePublicationDescriptionSnapshot(t, db, userID, "metadata only route", description)
	if snapshot.RouteDescription == nil || *snapshot.RouteDescription != description {
		t.Fatalf("snapshot route_description = %#v, want metadata copy", snapshot.RouteDescription)
	}

	publication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-no-guide-content",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse: %v", err)
	}

	var attachmentCount int
	if err := db.Pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM file_attachments
		WHERE entity_id IN ($1, $2)
		   OR entity_type IN ('route_guide', 'route_publication_guide', 'tour_report')
	`, snapshot.ID, publication.ID).Scan(&attachmentCount); err != nil {
		t.Fatalf("count file_attachments: %v", err)
	}
	if attachmentCount != 0 {
		t.Fatalf("guide/report attachment count = %d, want 0", attachmentCount)
	}
}

func createRoutePublicationTestSnapshot(t *testing.T, db *DB, userID, name string) (*model.PlannedRoute, *model.PlannedRouteSnapshot) {
	t.Helper()
	subType := "mountain"
	route := &model.PlannedRoute{
		UserID:          userID,
		Name:            name,
		SportType:       "cycling",
		SubType:         &subType,
		Profile:         "cycling-mountain",
		GeometryGeoJSON: `{"type":"LineString","coordinates":[[7.000,47.000],[7.001,47.001],[7.002,47.002]]}`,
		Waypoints: []model.RouteWaypoint{
			{ID: "a", Type: "start", LatLng: [2]float64{47.000, 7.000}},
			{ID: "b", Type: "end", LatLng: [2]float64{47.002, 7.002}},
		},
		Segments: []model.RouteSegment{
			{
				FromWaypoint: "a",
				ToWaypoint:   "b",
				Mode:         "routed",
				Smooth:       false,
				DistanceM:    300,
				Geometry:     &model.GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{7.000, 47.000}, {7.001, 47.001}, {7.002, 47.002}}},
			},
		},
		Enrichment:     []byte(`{"enrichment_version":1,"forest_pct":0.1,"protected_pct":0.2}`),
		TotalDistanceM: routePublicationPtrFloat64(300),
	}
	routeID, err := db.CreateRoute(context.Background(), route)
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	loaded, err := db.GetRoute(context.Background(), routeID, userID)
	if err != nil {
		t.Fatalf("GetRoute: %v", err)
	}
	snapshot, err := db.CreatePlannedRouteSnapshot(context.Background(), model.CreatePlannedRouteSnapshotInput{
		RouteID:            routeID,
		UserID:             userID,
		SourceRouteVersion: loaded.Version,
		OperationID:        "publication-snapshot-" + routeID,
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot: %v", err)
	}
	return loaded, snapshot
}

func createRoutePublicationDescriptionSnapshot(t *testing.T, db *DB, userID, name, description string) (*model.PlannedRoute, *model.PlannedRouteSnapshot) {
	t.Helper()
	route, _ := createRoutePublicationTestSnapshot(t, db, userID, name)
	route.Description = &description
	if err := db.UpdateRoute(context.Background(), route, route.Version); err != nil {
		t.Fatalf("UpdateRoute description: %v", err)
	}
	updated, err := db.GetRoute(context.Background(), route.ID, userID)
	if err != nil {
		t.Fatalf("GetRoute with description: %v", err)
	}
	snapshot, err := db.CreatePlannedRouteSnapshot(context.Background(), model.CreatePlannedRouteSnapshotInput{
		RouteID:            route.ID,
		UserID:             userID,
		SourceRouteVersion: updated.Version,
		OperationID:        "publication-description-snapshot-" + route.ID,
	})
	if err != nil {
		t.Fatalf("CreatePlannedRouteSnapshot with description: %v", err)
	}
	return updated, snapshot
}

func routePublicationPtrFloat64(v float64) *float64 {
	return &v
}
