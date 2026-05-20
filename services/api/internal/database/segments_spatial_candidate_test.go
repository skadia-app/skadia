package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
)

func TestSpatialCandidateSegmentsExcludeSpecializedEmptyPredicates(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()

	activityID := uuid.NewString()
	activityDate := time.Now().UTC().Add(24 * time.Hour)
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, 'running', '', false, false, false, 'Spatial Candidate Predicate Test', $3, $4, 'private', 'complete')`,
		activityID, userID, activityDate, "spatial-candidate-"+activityID,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		 VALUES (
		   $1,
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_Envelope(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
		   2
		 )`,
		activityID,
	); err != nil {
		t.Fatalf("insert activity track: %v", err)
	}

	nullWinterID := uuid.NewString()
	emptyWinterID := uuid.NewString()
	controlID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1)`, []string{nullWinterID, emptyWinterID, controlID})
	})

	insertCandidateSegment(t, db, userID, nullWinterID, "Winter Null", "generic", "wintersport", "")
	insertCandidateSegment(t, db, userID, emptyWinterID, "Winter Empty", "generic", "wintersport", "[]")
	insertCandidateSegment(t, db, userID, controlID, "Running Control", "foot", "standard", "")

	candidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, candidateBucketKey(t, "running", ""), activityDate)
	if err != nil {
		t.Fatalf("GetSpatialCandidateSegments: %v", err)
	}
	seen := map[string]CandidateSegment{}
	for _, candidate := range candidates {
		seen[candidate.SegmentID] = candidate
	}
	if _, ok := seen[controlID]; !ok {
		t.Fatalf("control segment %s missing from candidates: %#v", controlID, candidates)
	}
	if _, ok := seen[nullWinterID]; ok {
		t.Fatalf("wintersport NULL-predicate segment matched running candidates: %#v", candidates)
	}
	if _, ok := seen[emptyWinterID]; ok {
		t.Fatalf("wintersport empty-predicate segment matched running candidates: %#v", candidates)
	}
}

func TestSpatialCandidateSegmentsUsesUnboundedVersionValidity(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, activityDate := insertSpatialCandidateActivity(t, db, userID, "cycling")
	segmentID := uuid.NewString()
	insertCandidateSegment(t, db, userID, segmentID, "Unbounded Validity", "bike", "standard", "")

	if _, err := db.Pool.Exec(ctx, `
		UPDATE segment_versions
		SET valid_from_at = NULL, valid_until_at = NULL
		WHERE segment_id = $1`, segmentID); err != nil {
		t.Fatalf("set unbounded validity: %v", err)
	}

	candidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, candidateBucketKey(t, "cycling", ""), activityDate.Add(-365*24*time.Hour))
	if err != nil {
		t.Fatalf("GetSpatialCandidateSegments: %v", err)
	}
	if !candidateIDs(candidates)[segmentID] {
		t.Fatalf("unbounded segment missing from candidates: %#v", candidates)
	}
}

func TestSpatialCandidateSegmentsRejectsFutureOnlyVersionWithoutFallback(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, activityDate := insertSpatialCandidateActivity(t, db, userID, "cycling")
	segmentID := uuid.NewString()
	insertCandidateSegment(t, db, userID, segmentID, "Future Only", "bike", "standard", "")

	if _, err := db.Pool.Exec(ctx, `
		UPDATE segment_versions
		SET valid_from_at = $2, valid_until_at = NULL
		WHERE segment_id = $1`, segmentID, activityDate.Add(24*time.Hour)); err != nil {
		t.Fatalf("set future validity: %v", err)
	}

	candidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, candidateBucketKey(t, "cycling", ""), activityDate)
	if err != nil {
		t.Fatalf("GetSpatialCandidateSegments: %v", err)
	}
	if candidateIDs(candidates)[segmentID] {
		t.Fatalf("future-only segment matched via fallback: %#v", candidates)
	}
}

func TestSpatialCandidateSegmentsUseEligibilityProjectionParityCases(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, activityDate := insertSpatialCandidateActivity(t, db, userID, "cycling")

	bikeNullID := uuid.NewString()
	genericNullID := uuid.NewString()
	explicitMountainID := uuid.NewString()
	emptyID := uuid.NewString()
	winterNullID := uuid.NewString()
	genericFallbackID := uuid.NewString()
	bikeForGenericID := uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM segments WHERE id = ANY($1)`, []string{
			bikeNullID, genericNullID, explicitMountainID, emptyID, winterNullID, genericFallbackID, bikeForGenericID,
		})
	})

	insertCandidateSegment(t, db, userID, bikeNullID, "Bike Null", "bike", "standard", "")
	insertCandidateSegment(t, db, userID, genericNullID, "Generic Null", "generic", "standard", "")
	insertCandidateSegment(t, db, userID, explicitMountainID, "Mountain Only", "bike", "standard", `[{"sport":"cycling","sub_type":"mountain"}]`)
	insertCandidateSegment(t, db, userID, emptyID, "Empty Predicates", "bike", "standard", "[]")
	insertCandidateSegment(t, db, userID, winterNullID, "Winter Null", "generic", "wintersport", "")
	insertCandidateSegment(t, db, userID, genericFallbackID, "Generic Fallback", "generic", "standard", "")
	insertCandidateSegment(t, db, userID, bikeForGenericID, "Bike For Generic", "bike", "standard", "")

	mountainCandidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, candidateBucketKey(t, "cycling", "mountain"), activityDate)
	if err != nil {
		t.Fatalf("mountain candidates: %v", err)
	}
	mountainSeen := candidateIDs(mountainCandidates)
	for _, id := range []string{bikeNullID, genericNullID, explicitMountainID} {
		if !mountainSeen[id] {
			t.Fatalf("mountain candidate %s missing from %#v", id, mountainCandidates)
		}
	}
	for _, id := range []string{emptyID, winterNullID} {
		if mountainSeen[id] {
			t.Fatalf("rowless candidate %s appeared in %#v", id, mountainCandidates)
		}
	}

	roadCandidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, candidateBucketKey(t, "cycling", "road"), activityDate)
	if err != nil {
		t.Fatalf("road candidates: %v", err)
	}
	if candidateIDs(roadCandidates)[explicitMountainID] {
		t.Fatalf("explicit mountain predicate matched road bucket: %#v", roadCandidates)
	}

	futureKey, ok := sport.ActivityTraitBucketKeyForActivityLegacyCandidate("cycling", "future_bike", sport.ActivityModifierState{})
	if !ok {
		t.Fatalf("future cycling subtype did not fall back to parent bucket")
	}
	futureCandidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, futureKey, activityDate)
	if err != nil {
		t.Fatalf("future subtype candidates: %v", err)
	}
	if !candidateIDs(futureCandidates)[bikeNullID] {
		t.Fatalf("future cycling subtype did not match legacy bike segment: %#v", futureCandidates)
	}

	unknownKey, ok := sport.ActivityTraitBucketKeyForActivityLegacyCandidate("future_sport", "", sport.ActivityModifierState{})
	if !ok {
		t.Fatalf("unknown sport did not produce generic fallback bucket")
	}
	unknownCandidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, unknownKey, activityDate)
	if err != nil {
		t.Fatalf("unknown sport candidates: %v", err)
	}
	unknownSeen := candidateIDs(unknownCandidates)
	if !unknownSeen[genericFallbackID] {
		t.Fatalf("unknown sport did not match generic fallback segment: %#v", unknownCandidates)
	}
	if unknownSeen[bikeForGenericID] {
		t.Fatalf("unknown sport matched non-generic segment: %#v", unknownCandidates)
	}
}

func TestSpatialCandidateSegmentsUseEligibilityProjectionVisibility(t *testing.T) {
	db, userID := segmentPRTestDB(t)
	ctx := context.Background()
	activityID, activityDate := insertSpatialCandidateActivity(t, db, userID, "cycling")
	otherUserID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, otherUserID, "segment-visibility-"+otherUserID[:8], "segment-visibility-"+otherUserID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert other user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, otherUserID)
	})

	ownerPrivateID := uuid.NewString()
	otherPublicID := uuid.NewString()
	otherPrivateID := uuid.NewString()
	insertCandidateSegment(t, db, userID, ownerPrivateID, "Owner Private", "bike", "standard", "")
	insertCandidateSegment(t, db, otherUserID, otherPublicID, "Other Public", "bike", "standard", "")
	insertCandidateSegment(t, db, otherUserID, otherPrivateID, "Other Private", "bike", "standard", "")
	if _, err := db.Pool.Exec(ctx, `
		UPDATE segments
		SET visibility = CASE WHEN id = $1 THEN 'public' ELSE 'private' END
		WHERE id = ANY($2)
	`, otherPublicID, []string{ownerPrivateID, otherPublicID, otherPrivateID}); err != nil {
		t.Fatalf("set visibility fixtures: %v", err)
	}

	candidates, err := db.GetSpatialCandidateSegments(ctx, activityID, userID, candidateBucketKey(t, "cycling", ""), activityDate)
	if err != nil {
		t.Fatalf("visibility candidates: %v", err)
	}
	seen := candidateIDs(candidates)
	if !seen[ownerPrivateID] || !seen[otherPublicID] {
		t.Fatalf("owner private or public candidate missing: %#v", candidates)
	}
	if seen[otherPrivateID] {
		t.Fatalf("private segment from another user leaked into candidates: %#v", candidates)
	}
}

func insertCandidateSegment(t *testing.T, db *DB, userID, segmentID, name, sportGroup, segmentType, allowedPredicatesJSON string) {
	t.Helper()
	ctx := context.Background()
	versionID := uuid.NewString()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segments (id, created_by, name, sport_group, segment_type, allowed_predicates)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::jsonb)`,
		segmentID, userID, name, sportGroup, segmentType, allowedPredicatesJSON,
	); err != nil {
		t.Fatalf("insert segment %s: %v", name, err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		 VALUES ($1, $2, 1, ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326), CURRENT_DATE, $3)`,
		versionID, segmentID, userID,
	); err != nil {
		t.Fatalf("insert segment version %s: %v", name, err)
	}
	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("rebuild course eligibility projection %s: %v", name, err)
	}
}

func insertSpatialCandidateActivity(t *testing.T, db *DB, userID, sportType string) (string, time.Time) {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.NewString()
	activityDate := time.Now().UTC()
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, sub_type, is_electric, is_indoor, is_virtual, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, $3, '', false, false, false, 'Spatial Candidate Validity Test', $4, $5, 'private', 'complete')`,
		activityID, userID, sportType, activityDate, "spatial-candidate-validity-"+activityID); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		 VALUES (
		   $1,
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326),
		   ST_Envelope(ST_GeomFromText('LINESTRING(8 49, 8.01 49.01)', 4326)),
		   2
		 )`,
		activityID); err != nil {
		t.Fatalf("insert activity track: %v", err)
	}
	return activityID, activityDate
}

func candidateIDs(candidates []CandidateSegment) map[string]bool {
	ids := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		ids[candidate.SegmentID] = true
	}
	return ids
}

func candidateBucketKey(t *testing.T, sportType, subType string) string {
	t.Helper()
	key, ok := sport.ActivityTraitBucketKeyForActivityLegacyCandidate(sportType, subType, sport.ActivityModifierState{})
	if !ok {
		t.Fatalf("no activity trait bucket for %s/%s", sportType, subType)
	}
	return key
}
