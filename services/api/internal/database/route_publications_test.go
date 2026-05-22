package database

import (
	"context"
	"encoding/json"
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

func TestRoutePublicationRejectsInvalidEquivalenceAndOpenLap(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, snapshot := createRoutePublicationTestSnapshot(t, db, userID, "invalid route publication")

	_, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "continue_group",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-invalid-continue-new-course",
	})
	if !errors.Is(err, ErrRoutePublicationValidation) {
		t.Fatalf("new_course continue_group error = %v, want ErrRoutePublicationValidation", err)
	}

	_, err = db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "lap",
		OperationID:         "publish-invalid-open-lap",
	})
	if !errors.Is(err, ErrSegmentValidation) {
		t.Fatalf("open lap error = %v, want ErrSegmentValidation", err)
	}
}

func TestRoutePublicationExistingCoursePoliciesCreateVersions(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, firstSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "course v1")
	first, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     firstSnapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  firstSnapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-existing-course-v1",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse v1: %v", err)
	}

	_, secondSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "course v2")
	second, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:              secondSnapshot.ID,
		ActorUserID:                  userID,
		SourceRouteVersion:           secondSnapshot.SourceRouteVersion,
		PublishPolicy:                "new_version_same_course",
		Visibility:                   "private",
		TargetPublicationID:          &first.ID,
		TargetCourseSegmentID:        &first.CourseSegmentID,
		TargetCourseSegmentVersionID: &first.CourseSegmentVersionID,
		OperationID:                  "publish-existing-course-v2",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse v2: %v", err)
	}
	if second.CourseSegmentID != first.CourseSegmentID || second.CourseSegmentVersionID == first.CourseSegmentVersionID {
		t.Fatalf("existing-course publication = %#v, want same segment and new version after %#v", second, first)
	}
	if second.EquivalenceDecision != "version_specific" {
		t.Fatalf("equivalence decision = %q, want version_specific", second.EquivalenceDecision)
	}

	_, thirdSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "course v3")
	third, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:              thirdSnapshot.ID,
		ActorUserID:                  userID,
		SourceRouteVersion:           thirdSnapshot.SourceRouteVersion,
		PublishPolicy:                "continue_equivalence_group",
		Visibility:                   "private",
		TargetPublicationID:          &second.ID,
		TargetCourseSegmentID:        &second.CourseSegmentID,
		TargetCourseSegmentVersionID: &second.CourseSegmentVersionID,
		CompatibilityEvidence:        map[string]any{"length_delta_pct": 0.01},
		OperationID:                  "publish-existing-course-v3",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse v3: %v", err)
	}
	if third.CourseSegmentID != first.CourseSegmentID || third.EquivalenceDecision != "continue_group" {
		t.Fatalf("continued publication = %#v, want same course with continue_group", third)
	}

	var versionCount int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM segment_versions WHERE segment_id = $1`, first.CourseSegmentID).Scan(&versionCount); err != nil {
		t.Fatalf("count course versions: %v", err)
	}
	if versionCount != 3 {
		t.Fatalf("version count = %d, want 3", versionCount)
	}
}

func TestRoutePublicationContinueGroupBuildsTrustedCompatibilityEvidence(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, firstSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "trusted course v1")
	first, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     firstSnapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  firstSnapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-trusted-v1",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse v1: %v", err)
	}

	_, secondSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "trusted course v2")
	second, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:              secondSnapshot.ID,
		ActorUserID:                  userID,
		SourceRouteVersion:           secondSnapshot.SourceRouteVersion,
		PublishPolicy:                "continue_equivalence_group",
		Visibility:                   "private",
		TargetPublicationID:          &first.ID,
		TargetCourseSegmentID:        &first.CourseSegmentID,
		TargetCourseSegmentVersionID: &first.CourseSegmentVersionID,
		CompatibilityEvidence: map[string]any{
			"trusted":            true,
			"length_delta_ratio": 999,
		},
		OperationID: "publish-trusted-v2",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse v2: %v", err)
	}

	var evidence map[string]any
	if err := json.Unmarshal(second.CompatibilityEvidence, &evidence); err != nil {
		t.Fatalf("decode compatibility evidence: %v", err)
	}
	if evidence["trusted"] == true || evidence["length_delta_ratio"] == float64(999) {
		t.Fatalf("stored caller-supplied compatibility evidence: %s", second.CompatibilityEvidence)
	}
	for _, key := range []string{
		"source", "actor_user_id", "target_version_id", "new_version_id",
		"matching_contract_key", "matching_contract_version", "length_m",
		"target_length_m", "length_delta_ratio", "start_distance_m",
		"end_distance_m", "direction",
	} {
		if _, ok := evidence[key]; !ok {
			t.Fatalf("trusted evidence missing %q: %#v", key, evidence)
		}
	}
	if evidence["source"] != "server" {
		t.Fatalf("source = %#v, want server", evidence["source"])
	}

	replayed, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:              secondSnapshot.ID,
		ActorUserID:                  userID,
		SourceRouteVersion:           secondSnapshot.SourceRouteVersion,
		PublishPolicy:                "continue_equivalence_group",
		Visibility:                   "private",
		TargetPublicationID:          &first.ID,
		TargetCourseSegmentID:        &first.CourseSegmentID,
		TargetCourseSegmentVersionID: &first.CourseSegmentVersionID,
		CompatibilityEvidence: map[string]any{
			"trusted":            true,
			"length_delta_ratio": -999,
		},
		OperationID: "publish-trusted-v2",
	})
	if err != nil {
		t.Fatalf("replay with different caller evidence: %v", err)
	}
	if replayed.ID != second.ID {
		t.Fatalf("replay returned publication %s, want %s", replayed.ID, second.ID)
	}
}

func TestRouteDerivedCourseCannotBypassPublicationGateThroughSegmentPublish(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, snapshot := createRoutePublicationTestSnapshot(t, db, userID, "private route course")
	publication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "publish-private-course-for-segment-bypass",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse private: %v", err)
	}

	published, _, _, err := db.PublishSegmentAndSeedRecalculationRun(ctx, publication.CourseSegmentID, userID, false, "legacy-segment-publish-bypass", false, nil)
	if !errors.Is(err, ErrRoutePublicationValidation) {
		t.Fatalf("route-derived segment publish error = %v, want ErrRoutePublicationValidation", err)
	}
	if published {
		t.Fatal("route-derived segment publish returned published=true")
	}
}

func TestRoutePublicationPrivacyRejectsBlockedComponentStates(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, snapshot := createRoutePublicationTestSnapshot(t, db, userID, "privacy component route")
	if err := insertRoutePublicationReviewStates(t, db, snapshot.ID, snapshot.SnapshotHash, "public", "blocked", false); err != nil {
		t.Fatalf("insert privacy review: %v", err)
	}
	_, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "public",
		CompatibilityPreset: "standard",
		OperationID:         "publish-blocked-component",
	})
	if !errors.Is(err, ErrRoutePublicationValidation) {
		t.Fatalf("blocked component error = %v, want ErrRoutePublicationValidation", err)
	}
}

func TestRoutePublicationPrivacyAllowsOwnerConfirmedReviewState(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	_, snapshot := createRoutePublicationTestSnapshot(t, db, userID, "privacy confirmation route")

	if err := insertRoutePublicationReviewStates(t, db, snapshot.ID, snapshot.SnapshotHash, "public", "requires_confirmation", false); err != nil {
		t.Fatalf("insert unconfirmed review: %v", err)
	}
	input := model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "public",
		CompatibilityPreset: "standard",
		OperationID:         "publish-needs-owner-review",
	}
	if _, err := db.PublishRouteSnapshotAsCourse(ctx, input); !errors.Is(err, ErrRoutePublicationValidation) {
		t.Fatalf("unconfirmed review error = %v, want ErrRoutePublicationValidation", err)
	}

	if err := insertRoutePublicationReviewStates(t, db, snapshot.ID, snapshot.SnapshotHash, "public", "requires_confirmation", true); err != nil {
		t.Fatalf("replace confirmed review: %v", err)
	}
	input.OperationID = "publish-confirmed-review"
	if _, err := db.PublishRouteSnapshotAsCourse(ctx, input); err != nil {
		t.Fatalf("confirmed review publish: %v", err)
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

func insertRoutePublicationReviewStates(t *testing.T, db *DB, snapshotID, snapshotHash, audienceClass, state string, confirmed bool) error {
	t.Helper()
	ownerApproved := "NULL"
	if confirmed {
		ownerApproved = "NOW()"
	}
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO route_publication_privacy_reviews (
			route_snapshot_id, snapshot_hash, audience_class,
			route_name_state, description_state, waypoints_state,
			source_metadata_state, geometry_state, enrichment_state, media_pointer_state,
			overall_state, owner_approved_at, review_hash
		)
		VALUES (
			$1, $2, $3,
			$4, 'safe', 'safe',
			'safe', 'safe', 'safe', 'safe',
			$4, `+ownerApproved+`, $5
		)
		ON CONFLICT (route_snapshot_id, snapshot_hash, audience_class) DO UPDATE SET
			route_name_state = EXCLUDED.route_name_state,
			description_state = EXCLUDED.description_state,
			waypoints_state = EXCLUDED.waypoints_state,
			source_metadata_state = EXCLUDED.source_metadata_state,
			geometry_state = EXCLUDED.geometry_state,
			enrichment_state = EXCLUDED.enrichment_state,
			media_pointer_state = EXCLUDED.media_pointer_state,
			overall_state = EXCLUDED.overall_state,
			owner_approved_at = EXCLUDED.owner_approved_at,
			review_hash = EXCLUDED.review_hash,
			updated_at = NOW()
	`, snapshotID, snapshotHash, audienceClass, state, "review-"+state)
	return err
}

func routePublicationPtrFloat64(v float64) *float64 {
	return &v
}
