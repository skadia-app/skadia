package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
)

func TestRebuildSegmentCourseEligibilityProjectionExplicitPredicates(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	allowed := `[{"sport":"cycling","sub_type":"mountain"}]`
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", &allowed)

	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("RebuildSegmentCourseEligibilityProjection: %v", err)
	}

	mountainKey := sport.BuildActivityTraitBucketKey(sport.ActivityTraitBucketInput{SportType: "cycling", SubType: "mountain"})
	roadKey := sport.BuildActivityTraitBucketKey(sport.ActivityTraitBucketInput{SportType: "cycling", SubType: "road"})
	if !courseEligibilityRowExists(t, db, segmentID, mountainKey) {
		t.Fatalf("mountain cycling trait bucket missing")
	}
	if courseEligibilityRowExists(t, db, segmentID, roadKey) {
		t.Fatalf("road cycling trait bucket matched explicit mountain-only predicate")
	}
}

func TestRebuildSegmentCourseEligibilityProjectionNullLegacyWildcard(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)

	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("RebuildSegmentCourseEligibilityProjection: %v", err)
	}

	mountainKey := sport.BuildActivityTraitBucketKey(sport.ActivityTraitBucketInput{SportType: "cycling", SubType: "mountain"})
	if !courseEligibilityRowExists(t, db, segmentID, mountainKey) {
		t.Fatalf("legacy NULL predicates did not project bike-group rows")
	}
}

func TestRebuildSegmentCourseEligibilityProjectionEmptyPredicatesDeletesRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)
	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	if got := countCourseEligibilityRows(t, db, segmentID); got == 0 {
		t.Fatalf("initial rebuild produced no rows")
	}

	emptyPredicates := json.RawMessage(`[]`)
	if _, err := db.UpdateSegment(ctx, segmentID, userID, false, &model.SegmentUpdate{AllowedPredicates: emptyPredicates, NewVersion: true}); err != nil {
		t.Fatalf("set empty predicates: %v", err)
	}
	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("empty-predicate rebuild: %v", err)
	}
	if got := countLatestCourseEligibilityRows(t, db, segmentID); got != 0 {
		t.Fatalf("rows after empty predicates = %d, want 0", got)
	}
}

func TestRebuildSegmentCourseEligibilityProjectionWinterWaterNullDoesNotWildcard(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	for _, segmentType := range []string{"wintersport", "wassersport"} {
		t.Run(segmentType, func(t *testing.T) {
			segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, segmentType, "generic", nil)
			if err := db.RebuildSegmentCourseEligibilityProjection(context.Background(), segmentID); err != nil {
				t.Fatalf("RebuildSegmentCourseEligibilityProjection: %v", err)
			}
			if got := countCourseEligibilityRows(t, db, segmentID); got != 0 {
				t.Fatalf("%s NULL predicates projected %d wildcard rows, want 0", segmentType, got)
			}
		})
	}
}

func TestRebuildSegmentCourseEligibilityProjectionUpdatesAfterSegmentTypeChange(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)
	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}

	liftType := "lift"
	if _, err := db.UpdateSegment(ctx, segmentID, userID, false, &model.SegmentUpdate{SegmentType: &liftType, NewVersion: true}); err != nil {
		t.Fatalf("UpdateSegment: %v", err)
	}

	var rankAllowed bool
	if err := db.Pool.QueryRow(ctx, `
		SELECT COALESCE(bool_or(rank_allowed), false)
		FROM target_course_eligibility_projection
		WHERE target_kind = 'segment' AND target_id = $1 AND compatibility_preset = 'lift'
	`, segmentID).Scan(&rankAllowed); err != nil {
		t.Fatalf("query lift projection: %v", err)
	}
	if rankAllowed {
		t.Fatalf("lift projection kept rank_allowed=true")
	}
}

func TestRebuildSegmentCourseEligibilityProjectionDeletesRowsWhenSegmentDeleted(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)
	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("RebuildSegmentCourseEligibilityProjection: %v", err)
	}
	if got := countCourseEligibilityRows(t, db, segmentID); got == 0 {
		t.Fatalf("rebuild produced no rows")
	}

	if _, err := db.Pool.Exec(ctx, `DELETE FROM segments WHERE id = $1`, segmentID); err != nil {
		t.Fatalf("delete segment: %v", err)
	}
	if got := countCourseEligibilityRows(t, db, segmentID); got != 0 {
		t.Fatalf("rows after segment delete = %d, want 0", got)
	}
}

func TestRebuildSegmentCourseEligibilityProjectionRebuildsEveryExistingVersion(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 2, ST_GeomFromText('LINESTRING(8 49, 8.02 49.02)', 4326), CURRENT_DATE, $3)
	`, uuid.NewString(), segmentID, userID); err != nil {
		t.Fatalf("insert second version: %v", err)
	}

	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("RebuildSegmentCourseEligibilityProjection: %v", err)
	}
	if got := countProjectedVersions(t, db, segmentID); got != 2 {
		t.Fatalf("projected versions = %d, want 2", got)
	}
}

func TestUpdateSegmentNewVersionRebuildsCourseEligibilityProjection(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segment := createCourseEligibilitySegment(t, db, userID, "standard", "bike", nil)
	if got := countProjectedVersions(t, db, segment.ID); got != 1 {
		t.Fatalf("initial projected versions = %d, want 1", got)
	}

	if _, err := db.UpdateSegment(ctx, segment.ID, userID, false, &model.SegmentUpdate{NewVersion: true}); err != nil {
		t.Fatalf("UpdateSegment new version: %v", err)
	}
	if got := countProjectedVersions(t, db, segment.ID); got != 2 {
		t.Fatalf("projected versions after new version = %d, want 2", got)
	}
}

func TestUpdateSegmentNewVersionSnapshotsEligibilityAtVersionBoundary(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segment := createCourseEligibilitySegment(t, db, userID, "standard", "bike", nil)

	liftType := "lift"
	if _, err := db.UpdateSegment(ctx, segment.ID, userID, false, &model.SegmentUpdate{
		NewVersion:  true,
		SegmentType: &liftType,
	}); err != nil {
		t.Fatalf("UpdateSegment new lift version: %v", err)
	}

	v1 := segmentVersionIDByNumber(t, db, segment.ID, 1)
	v2 := segmentVersionIDByNumber(t, db, segment.ID, 2)
	if !courseEligibilityPresetExists(t, db, segment.ID, v1, "standard") {
		t.Fatalf("version 1 lost standard eligibility snapshot")
	}
	if courseEligibilityPresetExists(t, db, segment.ID, v1, "lift") {
		t.Fatalf("version 1 was rebuilt from later lift metadata")
	}
	if !courseEligibilityPresetExists(t, db, segment.ID, v2, "lift") {
		t.Fatalf("version 2 did not snapshot lift eligibility")
	}
	if courseEligibilityPresetExists(t, db, segment.ID, v2, "standard") {
		t.Fatalf("version 2 kept previous standard eligibility")
	}
	if courseEligibilityAnyRankAllowed(t, db, segment.ID, v2, "lift") {
		t.Fatalf("lift eligibility snapshot allowed ranking")
	}
}

func TestUpdateSegmentNewVersionRollsBackVersionWhenProjectionRebuildFails(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segment := createCourseEligibilitySegment(t, db, userID, "standard", "bike", nil)
	initialVersions := countSegmentVersions(t, db, segment.ID)

	if _, err := db.Pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION fail_course_eligibility_projection_insert()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION 'forced projection failure';
		END;
		$$;
		CREATE TRIGGER fail_course_eligibility_projection_insert
		BEFORE INSERT ON target_course_eligibility_projection
		FOR EACH ROW EXECUTE FUNCTION fail_course_eligibility_projection_insert();
	`); err != nil {
		t.Fatalf("install failing projection trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS fail_course_eligibility_projection_insert ON target_course_eligibility_projection;
			DROP FUNCTION IF EXISTS fail_course_eligibility_projection_insert();
		`)
	})

	if _, err := db.UpdateSegment(ctx, segment.ID, userID, false, &model.SegmentUpdate{NewVersion: true}); err == nil {
		t.Fatalf("UpdateSegment new version succeeded despite forced projection failure")
	}
	if got := countSegmentVersions(t, db, segment.ID); got != initialVersions {
		t.Fatalf("segment versions after failed new-version update = %d, want %d", got, initialVersions)
	}
}

func TestPublishSegmentRebuildsCourseEligibilityProjection(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)

	ok, err := db.PublishSegment(ctx, segmentID, userID, false)
	if err != nil {
		t.Fatalf("PublishSegment: %v", err)
	}
	if !ok {
		t.Fatalf("PublishSegment returned false")
	}
	if got := countCourseEligibilityRows(t, db, segmentID); got == 0 {
		t.Fatalf("publish did not rebuild course eligibility projection")
	}
}

func TestRebuildSegmentCourseEligibilityProjectionSeedsActivityTraitsBeforeRows(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	segmentID, _ := insertCourseEligibilitySegmentFixture(t, db, userID, "standard", "bike", nil)
	if _, err := db.Pool.Exec(ctx, `DELETE FROM target_course_eligibility_projection`); err != nil {
		t.Fatalf("clear course eligibility projection: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM activity_trait_projection`); err != nil {
		t.Fatalf("clear activity trait projection: %v", err)
	}

	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("RebuildSegmentCourseEligibilityProjection: %v", err)
	}
	var traits int
	if err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_trait_projection`).Scan(&traits); err != nil {
		t.Fatalf("count activity traits: %v", err)
	}
	if traits == 0 {
		t.Fatalf("activity traits were not seeded")
	}
	if got := countCourseEligibilityRows(t, db, segmentID); got == 0 {
		t.Fatalf("course eligibility rows were not inserted")
	}
}

func insertCourseEligibilitySegmentFixture(t *testing.T, db *DB, userID, segmentType, sportGroup string, allowedPredicatesJSON *string) (segmentID, versionID string) {
	t.Helper()
	ctx := context.Background()
	segmentID = uuid.NewString()
	versionID = uuid.NewString()
	var allowed any
	if allowedPredicatesJSON != nil {
		allowed = *allowedPredicatesJSON
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type, visibility, elevation_kind, allowed_predicates)
		VALUES ($1, $2, $3, $4, $5, 'private', 'hilly', $6::jsonb)
	`, segmentID, userID, "Course Eligibility "+segmentID, sportGroup, segmentType, allowed); err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, valid_from_at, created_by)
		VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, NULL, $3)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("insert segment version: %v", err)
	}
	return segmentID, versionID
}

func createCourseEligibilitySegment(t *testing.T, db *DB, userID, segmentType, sportGroup string, allowed []sport.Predicate) *model.Segment {
	t.Helper()
	segment, err := db.CreateSegment(context.Background(), userID, &model.SegmentCreate{
		Name:              "Course Eligibility Created " + uuid.NewString(),
		SportGroup:        sportGroup,
		SegmentType:       segmentType,
		GeoJSONTrack:      `{"type":"LineString","coordinates":[[8,49],[8.01,49.01]]}`,
		Visibility:        "private",
		AllowedPredicates: allowed,
	})
	if err != nil {
		encoded, _ := json.Marshal(allowed)
		t.Fatalf("CreateSegment allowed=%s: %v", encoded, err)
	}
	return segment
}

func countCourseEligibilityRows(t *testing.T, db *DB, segmentID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM target_course_eligibility_projection
		WHERE target_kind = 'segment' AND target_id = $1
	`, segmentID).Scan(&count); err != nil {
		t.Fatalf("count course eligibility rows: %v", err)
	}
	return count
}

func countLatestCourseEligibilityRows(t *testing.T, db *DB, segmentID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM target_course_eligibility_projection cep
		WHERE cep.target_kind = 'segment'
		  AND cep.target_id = $1
		  AND cep.target_version_id = (
			SELECT id
			FROM segment_versions
			WHERE segment_id = $1
			ORDER BY version_number DESC
			LIMIT 1
		  )
	`, segmentID).Scan(&count); err != nil {
		t.Fatalf("count latest course eligibility rows: %v", err)
	}
	return count
}

func countProjectedVersions(t *testing.T, db *DB, segmentID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(DISTINCT target_version_id)
		FROM target_course_eligibility_projection
		WHERE target_kind = 'segment' AND target_id = $1
	`, segmentID).Scan(&count); err != nil {
		t.Fatalf("count projected versions: %v", err)
	}
	return count
}

func countSegmentVersions(t *testing.T, db *DB, segmentID string) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM segment_versions
		WHERE segment_id = $1
	`, segmentID).Scan(&count); err != nil {
		t.Fatalf("count segment versions: %v", err)
	}
	return count
}

func courseEligibilityRowExists(t *testing.T, db *DB, segmentID, bucketKey string) bool {
	t.Helper()
	var exists bool
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM target_course_eligibility_projection
			WHERE target_kind = 'segment'
			  AND target_id = $1
			  AND activity_trait_bucket_key = $2
			  AND match_allowed
		)
	`, segmentID, bucketKey).Scan(&exists); err != nil {
		t.Fatalf("query course eligibility row: %v", err)
	}
	return exists
}

func courseEligibilityPresetExists(t *testing.T, db *DB, segmentID, versionID, preset string) bool {
	t.Helper()
	var exists bool
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM target_course_eligibility_projection
			WHERE target_kind = 'segment'
			  AND target_id = $1
			  AND target_version_id = $2
			  AND compatibility_preset = $3
		)
	`, segmentID, versionID, preset).Scan(&exists); err != nil {
		t.Fatalf("query course eligibility preset: %v", err)
	}
	return exists
}

func courseEligibilityAnyRankAllowed(t *testing.T, db *DB, segmentID, versionID, preset string) bool {
	t.Helper()
	var allowed bool
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT COALESCE(bool_or(rank_allowed), false)
		FROM target_course_eligibility_projection
		WHERE target_kind = 'segment'
		  AND target_id = $1
		  AND target_version_id = $2
		  AND compatibility_preset = $3
	`, segmentID, versionID, preset).Scan(&allowed); err != nil {
		t.Fatalf("query course eligibility rank_allowed: %v", err)
	}
	return allowed
}

func segmentVersionIDByNumber(t *testing.T, db *DB, segmentID string, versionNumber int) string {
	t.Helper()
	var versionID string
	if err := db.Pool.QueryRow(context.Background(), `
		SELECT id
		FROM segment_versions
		WHERE segment_id = $1 AND version_number = $2
	`, segmentID, versionNumber).Scan(&versionID); err != nil {
		t.Fatalf("query segment version %d: %v", versionNumber, err)
	}
	return versionID
}
