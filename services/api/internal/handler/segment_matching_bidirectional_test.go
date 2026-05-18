package handler

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// insertActivityWithTrack inserts a minimal activity row, an activity_tracks
// entry (track_full from WKT), and 5 activity_streams rows evenly spaced along
// the given coordinate slice.  Each stream row is 10 s apart so elapsed_time
// passes the minSegmentEffortTimeS = 5 s threshold.
//
// pts is a slice of [lat, lng] pairs ordered along the route.
// The caller must pass at least 5 pairs to satisfy the GPS-stream length guard
// in collectSegmentEfforts.
func insertActivityWithTrack(t *testing.T, pool *database.DB, userID, sportType, trackWKT string, pts [][2]float64) string {
	t.Helper()
	ctx := context.Background()

	var activityID string
	err := pool.Pool.QueryRow(ctx,
		`INSERT INTO activities (id, user_id, sport_type, started_at)
		 VALUES (gen_random_uuid(), $1, $2, NOW())
		 RETURNING id`,
		userID, sportType).Scan(&activityID)
	if err != nil {
		t.Fatalf("insertActivityWithTrack: insert activity: %v", err)
	}

	// Insert the track geometry (used by the spatial pre-filter JOIN).
	_, err = pool.Pool.Exec(ctx,
		`INSERT INTO activity_tracks (activity_id, track_full, track_simplified, bbox, point_count)
		 VALUES ($1,
		         ST_GeomFromText($2, 4326),
		         ST_GeomFromText($2, 4326),
		         ST_Envelope(ST_Expand(ST_GeomFromText($2, 4326), 0.0000001)),
		         $3)`,
		activityID, trackWKT, len(pts))
	if err != nil {
		t.Fatalf("insertActivityWithTrack: insert track: %v", err)
	}

	// Insert GPS stream rows (needed by GetActivityGPSStream → ≥5 points required).
	base := time.Now().UTC().Truncate(time.Second)
	for i, p := range pts {
		ts := base.Add(time.Duration(i*10) * time.Second)
		_, err = pool.Pool.Exec(ctx,
			`INSERT INTO activity_streams (time, activity_id, lat, lng)
			 VALUES ($1, $2, $3, $4)`,
			ts, activityID, p[0], p[1])
		if err != nil {
			t.Fatalf("insertActivityWithTrack: insert stream point %d: %v", i, err)
		}
	}

	t.Cleanup(func() {
		pool.Pool.Exec(context.Background(), "DELETE FROM activities WHERE id = $1", activityID)
	})
	return activityID
}

// TestMatcherSkipsReverseTraversal verifies that a non-bidirectional segment is
// not matched when an activity traverses it in the reverse direction, but IS
// matched in the forward direction.
//
// Segment: LINESTRING(0 0, 1 0) — West→East, Bidirectional=false.
// Forward activity:  GPS goes lng 0→1 (West→East) — should produce 1 effort.
// Reverse activity:  GPS goes lng 1→0 (East→West) — should produce 0 efforts.
func TestMatcherSkipsReverseTraversal(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}

	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track:         "LINESTRING(0 0, 1 0)",
		Bidirectional: boolPtr(false),
		SegmentType:   "standard",
	})

	ctx := context.Background()
	now := time.Now().UTC()

	// Forward activity: GPS stream West→East, matches segment forward direction.
	// Points at lat=0, lng=0..1 in 5 steps of 0.25.
	fwdPts := [][2]float64{
		{0, 0.00}, // lat=0, lng=0.00
		{0, 0.25},
		{0, 0.50},
		{0, 0.75},
		{0, 1.00},
	}
	fwdTrack := "LINESTRING(0 0, 0.25 0, 0.5 0, 0.75 0, 1 0)"
	fwdActID := insertActivityWithTrack(t, db, userID, "cycling", fwdTrack, fwdPts)

	MatchActivitySegments(ctx, db, fwdActID, userID, "cycling", "", nil, now)

	var fwdCount int
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2`,
		fwdActID, segmentID).Scan(&fwdCount)
	if fwdCount != 1 {
		t.Errorf("forward direction: segment_efforts count = %d, want 1", fwdCount)
	}
	var contractKey, matchIdentity, lbSport, lbDirection string
	var contractVersion int
	if err := pool.QueryRow(ctx, `
		SELECT matching_contract_key,
		       matching_contract_version,
		       match_identity_key,
		       leaderboard_sport_type,
		       leaderboard_direction
		FROM segment_efforts
		WHERE activity_id = $1 AND segment_id = $2`,
		fwdActID, segmentID).Scan(&contractKey, &contractVersion, &matchIdentity, &lbSport, &lbDirection); err != nil {
		t.Fatalf("query forward metadata: %v", err)
	}
	if contractKey != database.CourseGPSV1Key || contractVersion != database.CourseGPSV1Version {
		t.Fatalf("contract = %s/%d", contractKey, contractVersion)
	}
	if matchIdentity == "" {
		t.Fatal("match_identity_key is empty")
	}
	if lbSport != "cycling" || lbDirection != "forward" {
		t.Fatalf("leaderboard tuple = %s/%s, want cycling/forward", lbSport, lbDirection)
	}

	// Reverse activity: GPS stream East→West, should NOT match non-bidirectional segment.
	revPts := [][2]float64{
		{0, 1.00}, // lat=0, lng=1.00
		{0, 0.75},
		{0, 0.50},
		{0, 0.25},
		{0, 0.00},
	}
	revTrack := "LINESTRING(1 0, 0.75 0, 0.5 0, 0.25 0, 0 0)"
	revActID := insertActivityWithTrack(t, db, userID, "cycling", revTrack, revPts)

	MatchActivitySegments(ctx, db, revActID, userID, "cycling", "", nil, now)

	var revCount int
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM segment_efforts WHERE activity_id = $1 AND segment_id = $2`,
		revActID, segmentID).Scan(&revCount)
	if revCount != 0 {
		t.Errorf("reverse direction: segment_efforts count = %d, want 0 (non-bidirectional)", revCount)
	}
}

func TestMatcherAllowsReverseTraversalWhenBidirectional(t *testing.T) {
	pool := fusionTestPool(t)
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)
	segmentID := insertTestSegment(t, pool, userID, TestSegmentOpts{
		Track:         "LINESTRING(0 0, 1 0)",
		Bidirectional: boolPtr(true),
		SegmentType:   "standard",
	})

	ctx := context.Background()
	now := time.Now().UTC()
	reversePts := [][2]float64{
		{0, 1.00},
		{0, 0.75},
		{0, 0.50},
		{0, 0.25},
		{0, 0.00},
	}
	reverseTrack := "LINESTRING(1 0, 0.75 0, 0.5 0, 0.25 0, 0 0)"
	activityID := insertActivityWithTrack(t, db, userID, "cycling", reverseTrack, reversePts)

	MatchActivitySegments(ctx, db, activityID, userID, "cycling", "", nil, now)

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM segment_efforts
		WHERE activity_id = $1 AND segment_id = $2 AND is_reverse = true`,
		activityID, segmentID).Scan(&count); err != nil {
		t.Fatalf("count reverse efforts: %v", err)
	}
	if count != 1 {
		t.Fatalf("reverse effort count = %d, want 1", count)
	}
}
