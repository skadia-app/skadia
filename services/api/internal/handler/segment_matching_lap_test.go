package handler

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

func TestMatcherFreezesMetadataForLapEfforts(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	ctx := context.Background()

	var segmentID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO segments (
			id, name, sport_group, segment_type, segment_type_meta,
			created_by, visibility, bidirectional
		)
		VALUES (
			gen_random_uuid(), 'lap metadata test', 'generic', 'lap',
			'{"finish_lat":0,"finish_lng":0}'::jsonb,
			$1, 'public', true
		)
		RETURNING id`, userID).Scan(&segmentID); err != nil {
		t.Fatalf("insert lap segment: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from, created_by)
		VALUES (
			gen_random_uuid(), $1, 1,
			ST_GeomFromText('LINESTRING(0 0, 0.002 0, 0 0)', 4326),
			CURRENT_DATE, $2
		)`, segmentID, userID); err != nil {
		t.Fatalf("insert lap version: %v", err)
	}
	if err := db.RebuildSegmentCourseEligibilityProjection(ctx, segmentID); err != nil {
		t.Fatalf("rebuild course eligibility projection: %v", err)
	}

	pts := [][2]float64{
		{0, 0.000},
		{0, 0.001},
		{0, 0.002},
		{0, 0.001},
		{0, 0.000},
		{0, -0.001},
		{0, -0.002},
		{0, -0.001},
		{0, 0.000},
	}
	track := "LINESTRING(0 0, 0.001 0, 0.002 0, 0.001 0, 0 0, -0.001 0, -0.002 0, -0.001 0, 0 0)"
	activityID := insertActivityWithTrack(t, db, userID, "cycling", track, pts)
	if _, err := pool.Exec(ctx, `
		UPDATE activities
		SET is_indoor = true, is_virtual = true
		WHERE id = $1`, activityID); err != nil {
		t.Fatalf("mark activity indoor/virtual: %v", err)
	}

	MatchActivitySegments(ctx, db, activityID, userID, "cycling", "", nil, time.Now().UTC())

	var contractKey, matchIdentity, indoorState, virtualState, direction string
	if err := pool.QueryRow(ctx, `
		SELECT matching_contract_key,
		       match_identity_key,
		       leaderboard_indoor_policy_state,
		       leaderboard_virtual_policy_state,
		       leaderboard_direction
		FROM segment_efforts
		WHERE activity_id = $1 AND segment_id = $2
		LIMIT 1`, activityID, segmentID).Scan(&contractKey, &matchIdentity, &indoorState, &virtualState, &direction); err != nil {
		t.Fatalf("query lap effort metadata: %v", err)
	}
	if contractKey != database.CourseGPSV1Key {
		t.Fatalf("contract key = %s, want %s", contractKey, database.CourseGPSV1Key)
	}
	if matchIdentity == "" {
		t.Fatal("match_identity_key is empty")
	}
	if indoorState != "indoor" || virtualState != "virtual" || direction != "forward" {
		t.Fatalf("leaderboard metadata = indoor:%s virtual:%s direction:%s, want indoor/virtual/forward",
			indoorState, virtualState, direction)
	}
}
