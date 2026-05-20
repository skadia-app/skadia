package database

import (
	"context"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
)

func TestCourseEligibilityPreFlipBackfill_RebuildsExistingSegmentsAndVersions(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 2, ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326), CURRENT_DATE, $3)
	`, uuid.NewString(), segmentID, userID); err != nil {
		t.Fatalf("insert second version: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM target_course_eligibility_projection WHERE target_id = $1`, segmentID); err != nil {
		t.Fatalf("clear projection: %v", err)
	}

	if err := db.BackfillAndVerifyCourseEligibilityProjections(ctx); err != nil {
		t.Fatalf("BackfillAndVerifyCourseEligibilityProjections: %v", err)
	}
	if got := countProjectedVersions(t, db, segmentID); got != 2 {
		t.Fatalf("projected versions = %d, want 2", got)
	}
}

func TestCourseEligibilityPreFlipBackfill_FailsWhenMatchableLegacySegmentHasNoRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)
	if err := db.RebuildActivityTraitProjection(ctx); err != nil {
		t.Fatalf("RebuildActivityTraitProjection: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM target_course_eligibility_projection WHERE target_id = $1`, segmentID); err != nil {
		t.Fatalf("clear projection: %v", err)
	}

	err := db.VerifyCourseEligibilityProjectionCoverage(ctx)
	if err == nil {
		t.Fatalf("VerifyCourseEligibilityProjectionCoverage succeeded with missing matchable rows")
	}
	if !strings.Contains(err.Error(), "course eligibility projection missing") {
		t.Fatalf("coverage error = %v, want missing projection rows", err)
	}
}

func TestCourseEligibilityPreFlipBackfill_FailsWhenActivityTraitsAreMissing(t *testing.T) {
	db, _ := segmentPRTestDB(t)
	ctx := context.Background()
	if err := db.RebuildActivityTraitProjection(ctx); err != nil {
		t.Fatalf("RebuildActivityTraitProjection: %v", err)
	}
	t.Cleanup(func() {
		_ = db.RebuildActivityTraitProjection(context.Background())
	})
	buckets := sport.ActivityTraitBuckets()
	if len(buckets) == 0 {
		t.Fatalf("no generated activity trait buckets")
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM target_course_eligibility_projection`); err != nil {
		t.Fatalf("clear target projection: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM activity_trait_projection WHERE activity_trait_bucket_key = $1`, buckets[0].Key); err != nil {
		t.Fatalf("delete activity trait bucket: %v", err)
	}

	err := db.VerifyCourseEligibilityProjectionCoverage(ctx)
	if err == nil {
		t.Fatalf("VerifyCourseEligibilityProjectionCoverage succeeded with a missing activity trait")
	}
	if !strings.Contains(err.Error(), "activity trait projection missing generated bucket") {
		t.Fatalf("coverage error = %v, want missing activity trait bucket", err)
	}
}

func TestCourseEligibilityPreFlipBackfill_ParityForNullWildcardGroups(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentTypes := []string{"standard", "trail", "lap"}
	groups := []string{"bike", "foot", "swim", "ski", "paddle", "generic"}
	for _, segmentType := range segmentTypes {
		for _, group := range groups {
			t.Run(segmentType+"_"+group, func(t *testing.T) {
				segmentID, versionID := insertCourseEligibilitySegmentFixture(t, db, userID, segmentType, group, nil)
				if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
					t.Fatalf("RebuildSegmentCourseEligibilityProjection: %v", err)
				}
				got := countCourseEligibilityRows(t, db, segmentID)
				expected := len(sport.CourseEligibilityRowsForSegment(sport.CourseEligibilityInput{
					TargetKind:           "segment",
					TargetID:             segmentID,
					TargetVersionID:      versionID,
					SegmentType:          segmentType,
					SportGroup:           group,
					AllowedPredicatesSet: false,
					Bidirectional:        true,
					Visibility:           "private",
					ElevationKind:        "hilly",
					AdapterKey:           courseEligibilityAdapterKey,
					AdapterVersion:       courseEligibilityAdapterVersion,
				}))
				t.Logf("compatibility_preset=%s activity_group=%s rows=%d", segmentType, group, got)
				if got != expected {
					t.Fatalf("projection rows = %d, want %d", got, expected)
				}
			})
		}
	}
}
