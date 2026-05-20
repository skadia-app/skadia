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
		SET sac_max_rank = 2, mtb_max_rank = 1, distance_m = 300, elevation_gain_m = 20
		WHERE publication_id = $1
	`, first.ID); err != nil {
		t.Fatalf("seed first projection: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		UPDATE route_course_profile_projection
		SET sac_max_rank = 5, mtb_max_rank = 4, distance_m = 900, elevation_gain_m = 200
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
	hidden, err := db.ListPublishedRouteCourses(ctx, otherUserID, 20, 0, PublishedCourseFilters{VisibilityMode: "visible_to_me"})
	if err != nil {
		t.Fatalf("ListPublishedRouteCourses other: %v", err)
	}
	if len(hidden) != 0 {
		t.Fatalf("other user saw private publications: %#v", hidden)
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
