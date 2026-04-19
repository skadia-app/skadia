package handler

// segment_recompute_integration_test.go — integration tests for segment recompute fixes.
//
// Requires TEST_DATABASE_URL to be set; the whole file is skipped otherwise.
//
// What is being tested:
//
//	TestRecomputeEffortsForActivity_NoDuplicatesAcrossSegments (R3-1 + R4-2)
//	  Inserts a minimal activity with a real GPS stream traversing two
//	  overlapping test segments.  Calls RecomputeEffortsForActivity and
//	  verifies that each (segment_id, activity_id) ends up with at most 1
//	  effort row — no cross-segment duplication from the O(N) batched path.
//
//	  Pre-fix (before R3-1 filter in recomputeSegmentEffortsForActivity):
//	  running for seg1 would INSERT allEfforts (including seg2), leaving seg2
//	  with 2 rows.  Pre-fix (before R4-2 O(N) refactor): collectSegmentEfforts
//	  was called N times redundantly.  Both bugs are now guarded by this test.
//
//	TestRecomputeSegmentEffortsForActivity_TxFailureRollsBack (R2-10)
//	  Pre-seeds one segment_effort for (segment_id, activity_id).
//	  Injects a TxFactory that fails on the DELETE inside the transaction.
//	  Verifies that the pre-seeded effort is still present after the failed
//	  recompute — i.e. the DELETE was rolled back, not committed.
//
// Before R2-10: the DELETE was on Pool.Exec (auto-commit), so a failure *after*
// the DELETE would leave the pair with zero efforts.  The new implementation
// wraps both DELETE and INSERT in a single transaction, so any mid-tx failure
// preserves the pre-recompute state.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/google/uuid"
)

// TestRecomputeEffortsForActivity_NoDuplicatesAcrossSegments verifies that
// RecomputeEffortsForActivity (R4-2 O(N) path) does not produce duplicate
// effort rows for any (segment_id, activity_id) pair.
//
// Setup: two segments with identical tracks (LINESTRING 16.361 48.201 →
// 16.362 48.202) inside a Vienna bbox; the activity carries a GPS stream of 6
// points that traverses both segments.  collectSegmentEfforts is expected to
// return one effort per segment (same geometry → both matched).
//
// Invariant under test: after RecomputeEffortsForActivity, each
// (segment_id, activity_id) has ≤1 row in segment_efforts.
//
//	Pre-fix R3-1 (no SegmentID filter in recomputeSegmentEffortsForActivity):
//	Processing seg1 inserted ALL efforts (including seg2 → seg2 ends up 2×).
//
//	Pre-fix R4-2 (O(N²) loop): collectSegmentEfforts ran N times (identical
//	GPS-stream work repeated per segment) — no DB correctness bug, but the
//	per-segment INSERT was still sourced from the correctly-filtered slice,
//	so this test also guards against any regression in effort distribution.
func TestRecomputeEffortsForActivity_NoDuplicatesAcrossSegments(t *testing.T) {
	pool := fusionTestPool(t) // skips if TEST_DATABASE_URL is not set
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	h := &Handler{DB: db}
	userID := insertTestUser(t, pool)

	// Insert a minimal activity (sport_type=cycling, required by GetSpatialCandidateSegments
	// sport-group filter and segment sport_group='cycling' below).
	actID := insertMinimalActivity(t, db, userID)

	// Insert a real GPS stream: 6 points linearly interpolated along the segment
	// LINESTRING(16.361 48.201, 16.362 48.202), timestamped 3 s apart.
	// Requirements met:
	//   - ≥5 GPS points (collectSegmentEfforts bails at <5)
	//   - start point within endpointProximityM=40m of segment start (48.201, 16.361)
	//   - end point within endpointProximityM=40m of segment end   (48.202, 16.362)
	//   - elapsed time = 15 s ≥ minSegmentEffortTimeS=5
	//   - Hausdorff distance ≈ 0 (GPS track follows segment exactly)
	baseTime := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
	gpsPoints := []model.StreamPoint{
		{Time: baseTime, Lat: fusionPtrF64(48.2010), Lng: fusionPtrF64(16.3610)},
		{Time: baseTime.Add(3 * time.Second), Lat: fusionPtrF64(48.2012), Lng: fusionPtrF64(16.3612)},
		{Time: baseTime.Add(6 * time.Second), Lat: fusionPtrF64(48.2014), Lng: fusionPtrF64(16.3614)},
		{Time: baseTime.Add(9 * time.Second), Lat: fusionPtrF64(48.2016), Lng: fusionPtrF64(16.3616)},
		{Time: baseTime.Add(12 * time.Second), Lat: fusionPtrF64(48.2018), Lng: fusionPtrF64(16.3618)},
		{Time: baseTime.Add(15 * time.Second), Lat: fusionPtrF64(48.2020), Lng: fusionPtrF64(16.3620)},
	}
	insertStreams(t, db, actID, gpsPoints)

	// Build activity_tracks (track_full + bbox) from the GPS stream just inserted.
	// GetSpatialCandidateSegments requires track_full IS NOT NULL.
	if err := db.EnsureActivityTrack(ctx, actID); err != nil {
		t.Fatalf("EnsureActivityTrack: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM activity_tracks WHERE activity_id = $1", actID) //nolint:errcheck
	})

	// Insert two segments whose tracks both lie inside the bbox.
	insertTestSegment := func(name string) (segID, segVersionID string) {
		t.Helper()
		segID = uuid.New().String()
		_, err := pool.Exec(ctx, `
			INSERT INTO segments (id, created_by, name, sport_group, segment_type)
			VALUES ($1, $2, $3, 'cycling', 'standard')
		`, segID, userID, name)
		if err != nil {
			t.Fatalf("insert segment %s: %v", name, err)
		}
		segVersionID = uuid.New().String()
		_, err = pool.Exec(ctx, `
			INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from)
			VALUES ($1, $2, 1,
				ST_GeomFromText('LINESTRING(16.361 48.201, 16.362 48.202)', 4326),
				'2000-01-01')
		`, segVersionID, segID)
		if err != nil {
			t.Fatalf("insert segment_version %s: %v", name, err)
		}
		t.Cleanup(func() {
			pool.Exec(context.Background(), "DELETE FROM segments WHERE id = $1", segID) //nolint:errcheck
		})
		return segID, segVersionID
	}

	seg1ID, seg1VersionID := insertTestSegment("r3-1-seg-A")
	seg2ID, seg2VersionID := insertTestSegment("r3-1-seg-B")

	// Pre-seed one legacy effort per segment (simulates existing DB state before recompute).
	// RecomputeEffortsForActivity DELETEs these and replaces them with GPS-matched efforts.
	startedAt := time.Date(2024, 3, 1, 9, 0, 0, 0, time.UTC)
	for _, pair := range []struct{ segID, segVersionID string }{
		{seg1ID, seg1VersionID},
		{seg2ID, seg2VersionID},
	} {
		_, err := pool.Exec(ctx, `
			INSERT INTO segment_efforts
			  (segment_id, segment_version_id, activity_id, user_id,
			   started_at, elapsed_time_s, sport_type, sub_type)
			VALUES ($1, $2, $3, $4, $5, 120, 'cycling', 'generic')
		`, pair.segID, pair.segVersionID, actID, userID, startedAt)
		if err != nil {
			t.Fatalf("pre-seed effort for seg %s: %v", pair.segID, err)
		}
	}

	countEfforts := func(segID string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM segment_efforts WHERE segment_id = $1 AND activity_id = $2`,
			segID, actID,
		).Scan(&n); err != nil {
			t.Fatalf("count efforts seg %s: %v", segID, err)
		}
		return n
	}

	// Sanity: both start with exactly 1 effort.
	if n := countEfforts(seg1ID); n != 1 {
		t.Fatalf("pre-seed: want 1 effort for seg1, got %d", n)
	}
	if n := countEfforts(seg2ID); n != 1 {
		t.Fatalf("pre-seed: want 1 effort for seg2, got %d", n)
	}

	// RecomputeEffortsForActivity calls collectSegmentEfforts ONCE for the activity,
	// groups efforts by segment_id, then applies a per-segment transactional
	// DELETE + INSERT.  With the real GPS stream the matcher should produce one
	// effort per segment (both have the same LINESTRING, both get traversed).
	//
	// Pre-fix R3-1 scenario (no SegmentID filter): processing seg1 would INSERT
	// allEfforts (including seg2 entries) → seg2 ends up with 2 rows.
	// Pre-fix R4-2 scenario (O(N²)): collectSegmentEfforts ran N times, but the
	// INSERT-slice was still filtered (R3-1 already applied), so no correctness
	// regression in that dimension — both are guarded by the ≤1 assertion below.
	if err := h.RecomputeEffortsForActivity(ctx, actID, userID); err != nil {
		t.Fatalf("RecomputeEffortsForActivity: %v", err)
	}

	// After recompute: pre-seeded efforts were DELETEd; GPS-matched efforts were
	// INSERTed (0 or 1 per segment depending on whether the segment matcher fires).
	// The critical invariant: NO segment may have >1 effort for this activity.
	// >1 would indicate either the R3-1 cross-segment duplication bug or a new
	// regression in the R4-2 batched path.
	for _, pair := range []struct {
		segID string
		name  string
	}{
		{seg1ID, "seg1"},
		{seg2ID, "seg2"},
	} {
		n := countEfforts(pair.segID)
		if n > 1 {
			t.Errorf("duplication detected (R3-1/R4-2 regression): %s has %d efforts for activity %s, want ≤1",
				pair.name, n, actID)
		}
	}
}

func TestRecomputeSegmentEffortsForActivity_TxFailureRollsBack(t *testing.T) {
	pool := fusionTestPool(t) // skips if TEST_DATABASE_URL is not set
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)

	// Insert a minimal activity.
	actID := insertMinimalActivity(t, db, userID)

	// Insert a minimal segment.
	segID := uuid.New().String()
	_, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		VALUES ($1, $2, 'r2-10-rollback-test', 'cycling', 'standard')
	`, segID, userID)
	if err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM segments WHERE id = $1", segID) //nolint:errcheck
	})

	// Insert a minimal segment_version so the FK on segment_efforts is satisfied.
	segVersionID := uuid.New().String()
	_, err = pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from)
		VALUES ($1, $2, 1,
			ST_GeomFromText('LINESTRING(16.361 48.201, 16.362 48.202)', 4326),
			'2000-01-01')
	`, segVersionID, segID)
	if err != nil {
		t.Fatalf("insert segment_version: %v", err)
	}

	// Pre-seed one segment_effort for (segment_id, activity_id).
	// This is the effort that must survive the failed recompute.
	startedAt := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `
		INSERT INTO segment_efforts
		  (segment_id, segment_version_id, activity_id, user_id,
		   started_at, elapsed_time_s, sport_type, sub_type)
		VALUES ($1, $2, $3, $4, $5, 120, 'cycling', 'generic')
	`, segID, segVersionID, actID, userID, startedAt)
	if err != nil {
		t.Fatalf("pre-seed segment_effort: %v", err)
	}

	// Verify pre-seed is there.
	assertEffortCount := func(wantCount int) {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM segment_efforts WHERE segment_id = $1 AND activity_id = $2`,
			segID, actID,
		).Scan(&count); err != nil {
			t.Fatalf("count segment_efforts: %v", err)
		}
		if count != wantCount {
			t.Errorf("segment_efforts count: got %d, want %d", count, wantCount)
		}
	}
	assertEffortCount(1)

	// Inject a TxFactory that fails on the DELETE inside the tx.
	// This simulates any mid-transaction failure that would previously leave
	// the (segment_id, activity_id) pair with zero efforts.
	injectedErr := fmt.Errorf("injected: DELETE failure for R2-10 rollback test")
	h := &Handler{
		DB: db,
		TxFactory: func(txCtx context.Context) (pgx.Tx, error) {
			realTx, err := db.Pool.Begin(txCtx)
			if err != nil {
				return nil, err
			}
			return &mockTx{
				Tx:          realTx,
				shouldFail:  sqlContains("delete from segment_efforts"),
				injectedErr: injectedErr,
			}, nil
		},
	}

	// recomputeSegmentEffortsForActivity must return an error (the injected one).
	err = h.recomputeSegmentEffortsForActivity(ctx, segID, actID, userID)
	if err == nil {
		t.Fatal("expected error from recomputeSegmentEffortsForActivity, got nil")
	}

	// The pre-seeded effort must still be present — the DELETE was rolled back.
	assertEffortCount(1)
}

