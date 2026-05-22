package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestRoutePublicationGDPRDeleteRemovesPublicationMetadata(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)

	scope, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: userID,
		ScopeKind:   "manual",
		CreatedBy:   userID,
		Grants: []model.ObjectAccessGrantInput{
			{PrincipalKind: "user", PrincipalUserID: &viewerID},
			{PrincipalKind: "link_token", LinkTokenHash: new("sha256:gdpr-route-publication-token")},
		},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}

	firstRoute, firstSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "gdpr first route")
	secondRoute, secondSnapshot := createRoutePublicationTestSnapshot(t, db, userID, "gdpr second route")
	insertRoutePublicationPrivacyReview(t, db, firstSnapshot.ID, firstSnapshot.SnapshotHash, "shared")

	firstPublication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     firstSnapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  firstSnapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "shared",
		AccessScopeID:       &scope.ID,
		CompatibilityPreset: "standard",
		OperationID:         "gdpr-publish-shared",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse shared: %v", err)
	}
	secondPublication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     secondSnapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  secondSnapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "private",
		CompatibilityPreset: "standard",
		OperationID:         "gdpr-publish-private",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse private: %v", err)
	}
	if _, err := db.CreateRouteRelationship(ctx, firstRoute.ID, secondRoute.ID, userID, "variant_of", "gdpr-route-rel", "hash:gdpr-route-rel"); err != nil {
		t.Fatalf("CreateRouteRelationship: %v", err)
	}
	if _, err := db.CreateRouteCourseRelationship(ctx, firstPublication.ID, secondPublication.ID, userID, "variant_of", "gdpr-course-rel", "hash:gdpr-course-rel"); err != nil {
		t.Fatalf("CreateRouteCourseRelationship: %v", err)
	}

	if _, err := db.Pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1`, userID); err != nil {
		t.Fatalf("mark user deleted: %v", err)
	}
	if err := db.DeleteUserData(ctx, userID, nil); err != nil {
		t.Fatalf("DeleteUserData: %v", err)
	}

	for _, tc := range []struct {
		name  string
		query string
	}{
		{"planned_route_snapshots", `SELECT COUNT(*) FROM planned_route_snapshots WHERE id IN ($1, $2)`},
		{"route_publication_privacy_reviews", `SELECT COUNT(*) FROM route_publication_privacy_reviews WHERE route_snapshot_id IN ($1, $2)`},
		{"route_profile_snapshots", `SELECT COUNT(*) FROM route_profile_snapshots WHERE route_snapshot_id IN ($1, $2)`},
	} {
		assertRoutePublicationGDPRCount(t, db, tc.name, tc.query, firstSnapshot.ID, secondSnapshot.ID)
	}
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"route_course_publications", `SELECT COUNT(*) FROM route_course_publications WHERE id IN ($1, $2)`},
		{"route_course_profile_projection", `SELECT COUNT(*) FROM route_course_profile_projection WHERE publication_id IN ($1, $2)`},
		{"route_course_relationships", `SELECT COUNT(*) FROM route_course_relationships WHERE source_publication_id = $1 OR target_publication_id = $2`},
	} {
		assertRoutePublicationGDPRCount(t, db, tc.name, tc.query, firstPublication.ID, secondPublication.ID)
	}
	assertRoutePublicationGDPRCount(t, db, "route_relationships", `SELECT COUNT(*) FROM route_relationships WHERE source_route_id = $1 OR target_route_id = $2`, firstRoute.ID, secondRoute.ID)
	assertRoutePublicationGDPRCount(t, db, "object_access_scopes", `SELECT COUNT(*) FROM object_access_scopes WHERE id = $1`, scope.ID)
	assertRoutePublicationGDPRCount(t, db, "object_access_grants", `SELECT COUNT(*) FROM object_access_grants WHERE access_scope_id = $1 OR principal_user_id = $2`, scope.ID, viewerID)
	assertRoutePublicationGDPRCount(t, db, "route_publication_events actor", `SELECT COUNT(*) FROM route_publication_events WHERE actor_user_id = $1`, userID)
}

func TestPublishedCourseSharedScopeRevocationHidesSearch(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)

	scope, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: userID,
		ScopeKind:   "manual",
		CreatedBy:   userID,
		Grants:      []model.ObjectAccessGrantInput{{PrincipalKind: "user", PrincipalUserID: &viewerID}},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	_, snapshot := createRoutePublicationTestSnapshot(t, db, userID, "shared searchable route")
	insertRoutePublicationPrivacyReview(t, db, snapshot.ID, snapshot.SnapshotHash, "shared")
	publication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         userID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "shared",
		AccessScopeID:       &scope.ID,
		CompatibilityPreset: "standard",
		OperationID:         "shared-search-before-revoke",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse: %v", err)
	}

	before, err := db.ListPublishedRouteCourses(ctx, viewerID, 20, 0, PublishedCourseFilters{VisibilityMode: "shared_visible_to_me"})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses before revoke: %v", err)
	}
	if len(before) != 1 || before[0].PublicationID != publication.ID {
		t.Fatalf("viewer courses before revoke = %#v, want %s", before, publication.ID)
	}
	if err := db.RevokeObjectAccessScope(ctx, scope.ID, userID); err != nil {
		t.Fatalf("RevokeObjectAccessScope: %v", err)
	}
	after, err := db.ListPublishedRouteCourses(ctx, viewerID, 20, 0, PublishedCourseFilters{VisibilityMode: "shared_visible_to_me"})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses after revoke: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("viewer courses after revoke = %#v, want none", after)
	}
}

func insertRoutePublicationPrivacyReview(t *testing.T, db *DB, snapshotID, snapshotHash, audienceClass string) {
	t.Helper()
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO route_publication_privacy_reviews (
			route_snapshot_id, snapshot_hash, audience_class,
			route_name_state, description_state, waypoints_state, source_metadata_state,
			geometry_state, enrichment_state, media_pointer_state, overall_state,
			review_hash
		)
		VALUES ($1, $2, $3, 'safe', 'safe', 'safe', 'safe', 'safe', 'safe', 'safe', 'safe', $4)
		ON CONFLICT (route_snapshot_id, snapshot_hash, audience_class)
		DO UPDATE SET overall_state = EXCLUDED.overall_state
	`, snapshotID, snapshotHash, audienceClass, "hash:"+snapshotID+":"+audienceClass); err != nil {
		t.Fatalf("insert route publication privacy review: %v", err)
	}
}

func assertRoutePublicationGDPRCount(t *testing.T, db *DB, name, query string, args ...any) {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", name, err)
	}
	if count != 0 {
		t.Fatalf("%s count = %d, want 0", name, count)
	}
}
