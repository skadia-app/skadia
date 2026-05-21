package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestPublishedCourseProfileFilters(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	first := publishSearchTestCourse(t, db, userID, "filter easy", "search-publish-1")
	second := publishSearchTestCourse(t, db, userID, "filter hard", "search-publish-2")

	if _, err := db.Pool.Exec(ctx, `
		UPDATE route_course_profile_projection
		SET sport_type = 'test_search_profile_filters', sac_max_rank = 2, mtb_max_rank = 1, distance_m = 300, elevation_gain_m = 20
		WHERE publication_id = $1
	`, first.ID); err != nil {
		t.Fatalf("seed first projection: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE route_course_profile_projection
		SET sport_type = 'test_search_profile_filters', sac_max_rank = 5, mtb_max_rank = 4, distance_m = 900, elevation_gain_m = 200
		WHERE publication_id = $1
	`, second.ID); err != nil {
		t.Fatalf("seed second projection: %v", err)
	}
	if _, err := db.CreateRouteCourseRelationship(ctx, first.ID, second.ID, userID, "variant_of", "search-variant", "hash-search-variant"); err != nil {
		t.Fatalf("CreateRouteCourseRelationship: %v", err)
	}

	rank := 3
	maxDist := 400.0
	withVariants := true
	got, err := db.ListPublishedRouteCourses(ctx, userID, 20, 0, PublishedCourseFilters{
		SACMaxRank:     &rank,
		MaxDistanceM:   &maxDist,
		HasVariants:    &withVariants,
		VisibilityMode: "owner",
		SportType:      "test_search_profile_filters",
	})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("result count = %d, want 1: %#v", len(got), got)
	}
	if got[0].PublicationID != first.ID {
		t.Fatalf("publication id = %s, want %s", got[0].PublicationID, first.ID)
	}
	if !got[0].HasVariants {
		t.Fatal("has_variants = false, want true")
	}

	otherUserID := insertObjectAccessTestUser(t, db)
	hidden, err := db.ListPublishedRouteCourses(ctx, otherUserID, 20, 0, PublishedCourseFilters{
		VisibilityMode: "visible_to_me",
		SportType:      "test_search_profile_filters",
	})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses other: %v", err)
	}
	if len(hidden) != 0 {
		t.Fatalf("other user saw private publications: %#v", hidden)
	}
}

func TestPublishedCourseFiltersExcludeUnknownDifficulty(t *testing.T) {
	db, userID := routeSubTypeTestDB(t)
	ctx := context.Background()
	ranked := publishSearchTestCourse(t, db, userID, "ranked", "search-ranked")
	unknown := publishSearchTestCourse(t, db, userID, "unknown", "search-unknown")

	if _, err := db.Pool.Exec(ctx, `
		UPDATE route_course_profile_projection
		SET sac_max_rank = 2
		WHERE publication_id = $1
	`, ranked.ID); err != nil {
		t.Fatalf("seed ranked projection: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE route_course_profile_projection
		SET sac_max_rank = NULL
		WHERE publication_id = $1
	`, unknown.ID); err != nil {
		t.Fatalf("seed unknown projection: %v", err)
	}

	rank := 3
	got, err := db.ListPublishedRouteCourses(ctx, userID, 20, 0, PublishedCourseFilters{
		SACMaxRank:     &rank,
		VisibilityMode: "owner",
	})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses: %v", err)
	}
	if len(got) != 1 || got[0].PublicationID != ranked.ID {
		t.Fatalf("difficulty filter result = %#v, want ranked only", got)
	}
}

func TestPublishedCourseVariantsRespectRelatedVisibility(t *testing.T) {
	db, ownerID := routeSubTypeTestDB(t)
	ctx := context.Background()
	viewerID := insertObjectAccessTestUser(t, db)
	publicCourse := publishSearchTestCourse(t, db, ownerID, "public variant", "search-public-variant")
	privateVariant := publishSearchTestCourse(t, db, ownerID, "private variant", "search-private-variant")
	sportType := "test_search_variant_visibility_" + publicCourse.ID[:8]
	if _, err := db.Pool.Exec(ctx, `UPDATE segments SET visibility = 'public' WHERE id = $1`, publicCourse.CourseSegmentID); err != nil {
		t.Fatalf("make course public: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE route_course_profile_projection
		SET sport_type = $2
		WHERE publication_id = ANY($1::uuid[])
	`, []string{publicCourse.ID, privateVariant.ID}, sportType); err != nil {
		t.Fatalf("tag variant projections: %v", err)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin sync projection: %v", err)
	}
	if err := syncPublishedCourseProjectionAccessTx(ctx, tx, publicCourse.CourseSegmentID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("sync public projection: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit sync projection: %v", err)
	}
	if _, err := db.CreateRouteCourseRelationship(ctx, publicCourse.ID, privateVariant.ID, ownerID, "variant_of", "search-private-variant-rel", "hash-private-variant-rel"); err != nil {
		t.Fatalf("CreateRouteCourseRelationship: %v", err)
	}

	got, err := db.ListPublishedRouteCourses(ctx, viewerID, 20, 0, PublishedCourseFilters{
		VisibilityMode: "public_only",
		SportType:      sportType,
	})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses viewer: %v", err)
	}
	if len(got) != 1 || got[0].PublicationID != publicCourse.ID {
		t.Fatalf("viewer public result = %#v, want public course only", got)
	}
	if got[0].HasVariants {
		t.Fatalf("has_variants leaked private variant: %#v", got[0])
	}
}

func TestPublishedCourseHasVariantsIgnoresDerivedFrom(t *testing.T) {
	db, ownerID := routeSubTypeTestDB(t)
	ctx := context.Background()
	first := publishSearchTestCourse(t, db, ownerID, "derived source", "search-derived-source")
	second := publishSearchTestCourse(t, db, ownerID, "derived target", "search-derived-target")
	if _, err := db.CreateRouteCourseRelationship(ctx, first.ID, second.ID, ownerID, "derived_from", "search-derived-rel", "hash-derived-rel"); err != nil {
		t.Fatalf("CreateRouteCourseRelationship derived_from: %v", err)
	}

	got, err := db.ListPublishedRouteCourses(ctx, ownerID, 20, 0, PublishedCourseFilters{VisibilityMode: "owner"})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses: %v", err)
	}
	for _, course := range got {
		if course.PublicationID == first.ID && course.HasVariants {
			t.Fatalf("derived_from relationship set has_variants: %#v", course)
		}
	}
}

func TestPublishedCourseVisibilityModesAreSpecific(t *testing.T) {
	db, ownerID := routeSubTypeTestDB(t)
	ctx := context.Background()
	privateCourse := publishSearchTestCourse(t, db, ownerID, "private course", "search-private-mode")
	publicCourse := publishSearchTestCourse(t, db, ownerID, "public course", "search-public-mode")
	if _, err := db.Pool.Exec(ctx, `UPDATE segments SET visibility = 'public' WHERE id = $1`, publicCourse.CourseSegmentID); err != nil {
		t.Fatalf("make course public: %v", err)
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin sync projection: %v", err)
	}
	if err := syncPublishedCourseProjectionAccessTx(ctx, tx, publicCourse.CourseSegmentID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("sync public projection: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit sync projection: %v", err)
	}

	got, err := db.ListPublishedRouteCourses(ctx, ownerID, 20, 0, PublishedCourseFilters{VisibilityMode: "owner_private"})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses owner_private: %v", err)
	}
	if len(got) != 1 || got[0].PublicationID != privateCourse.ID {
		t.Fatalf("owner_private result = %#v, want private course only", got)
	}
}

func TestPublishedCourseSharedSearchAcceptsResolvedAccessScopes(t *testing.T) {
	db, ownerID := routeSubTypeTestDB(t)
	ctx := context.Background()
	linkHash := "sha256:published-course-link-token"
	scope, err := db.CreateObjectAccessScope(ctx, model.ObjectAccessScopeInput{
		OwnerUserID: ownerID,
		ScopeKind:   "link",
		CreatedBy:   ownerID,
		Grants:      []model.ObjectAccessGrantInput{{PrincipalKind: "link_token", LinkTokenHash: &linkHash}},
	})
	if err != nil {
		t.Fatalf("CreateObjectAccessScope: %v", err)
	}
	_, snapshot := createRoutePublicationTestSnapshot(t, db, ownerID, "link shared course")
	sportType := "test_search_link_" + scope.ID[:8]
	insertRoutePublicationPrivacyReview(t, db, snapshot.ID, snapshot.SnapshotHash, "shared")
	publication, err := db.PublishRouteSnapshotAsCourse(ctx, model.PublishRouteSnapshotInput{
		RouteSnapshotID:     snapshot.ID,
		ActorUserID:         ownerID,
		SourceRouteVersion:  snapshot.SourceRouteVersion,
		PublishPolicy:       "new_course",
		EquivalenceDecision: "version_specific",
		Visibility:          "shared",
		AccessScopeID:       &scope.ID,
		CompatibilityPreset: "standard",
		OperationID:         "search-link-shared-course",
	})
	if err != nil {
		t.Fatalf("PublishRouteSnapshotAsCourse shared: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE route_course_profile_projection
		SET sport_type = $2
		WHERE publication_id = $1
	`, publication.ID, sportType); err != nil {
		t.Fatalf("tag shared projection: %v", err)
	}
	resolved, err := db.ResolveVisibleAccessScopes(ctx, "", &linkHash, nil)
	if err != nil {
		t.Fatalf("ResolveVisibleAccessScopes: %v", err)
	}

	got, err := db.ListPublishedRouteCourses(ctx, "", 20, 0, PublishedCourseFilters{VisibilityMode: "shared_visible_to_me", SportType: sportType, VisibleAccessScopeIDs: resolved})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses with scope: %v", err)
	}
	if len(got) != 1 || got[0].PublicationID != publication.ID {
		t.Fatalf("shared link result = %#v, want %s", got, publication.ID)
	}
}

func publishSearchTestCourse(t *testing.T, db *DB, userID, name, operationID string) *model.RouteCoursePublication {
	t.Helper()
	_, snapshot := createRoutePublicationTestSnapshot(t, db, userID, name)
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
