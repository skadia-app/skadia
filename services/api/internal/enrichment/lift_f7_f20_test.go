package enrichment

// Tests for Critic findings F7 and F20.
//
// F7: both-toggles-off cleanup — enrichStreamLiftsOpts with both heuristic
// and OSM-snap disabled must DELETE existing non-rejected candidates
// (and their segment_efforts) before returning.
//
// F20: nil-rides safety — enrichStreamLiftsOpts with heuristic=false and
// OSM-snap=true must not panic when rides is nil; it routes through the
// pure-OSM path (MethodACoverageGapForAllOSM).

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestEnrichStreamLifts_BothOff_CleansUpStaleCandidates (F7) seeds two
// non-rejected activity_lift_candidates for a ski activity, then calls
// enrichStreamLiftsOpts with both heuristic and OSM-snap forced off via
// per-activity overrides. Both candidates must be deleted; the one soft-
// rejected row must survive.
func TestEnrichStreamLifts_BothOff_CleansUpStaleCandidates(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()
	userID := liftTestUser(t, pool)

	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash,
		                        lift_detection_osm_override, lift_detection_heuristic_override)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5, false, false)
	`, activityID, userID, now, now.Add(time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id = $1", activityID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE id = $1", activityID)                        //nolint:errcheck
	})

	// Seed a non-rejected candidate (should be cleaned up).
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', false, 'heuristic')
	`, activityID, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("seed stale candidate: %v", err)
	}

	// Seed a soft-rejected candidate (must survive cleanup).
	rejStart := now.Add(2 * time.Minute)
	rejEnd := now.Add(3 * time.Minute)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 60, 80, 0, 500, 'medium', false, 'heuristic', true)
	`, activityID, rejStart, rejEnd); err != nil {
		t.Fatalf("seed rejected candidate: %v", err)
	}

	// Both overrides are already set to false on the activity row, so master=true,
	// user OSM=true, user heuristic=true, overrides=false/false → opts={false,false}.
	fTrue := true
	settings := &model.EnrichmentSettings{
		EnableLiftDetection:          true,
		EnableLiftDetectionOSM:       true,
		EnableLiftDetectionHeuristic: true,
	}
	_ = fTrue

	db := &database.DB{Pool: pool}
	stream := []EnrichmentStreamPoint{}
	if err := enrichStreamLiftsCore(ctx, db, userID, activityID, stream, false, false, settings); err != nil {
		t.Fatalf("enrichStreamLiftsCore: %v", err)
	}

	// The non-rejected candidate must be gone.
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM activity_lift_candidates
		WHERE activity_id = $1 AND NOT rejected_by_user
	`, activityID).Scan(&count); err != nil {
		t.Fatalf("count non-rejected: %v", err)
	}
	if count != 0 {
		t.Errorf("non-rejected candidate count = %d, want 0 (both-off must clean up)", count)
	}

	// The soft-rejected candidate must still be there.
	var rejCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM activity_lift_candidates
		WHERE activity_id = $1 AND rejected_by_user = true
	`, activityID).Scan(&rejCount); err != nil {
		t.Fatalf("count rejected: %v", err)
	}
	if rejCount != 1 {
		t.Errorf("rejected_by_user count = %d, want 1 (soft-rejected must survive)", rejCount)
	}
}

// TestEnrichStreamLifts_HeuristicOff_OSMOn_NilRidesSafe (F20) verifies that
// enrichStreamLiftsOpts does not panic when heuristic=false and OSM-snap=true.
// In this mode rides is nil; the pipeline routes through MethodACoverageGapForAllOSM
// without dereferencing the rides slice. With no OSM data in the test DB the
// function short-circuits before any OSM candidate work and must return nil.
func TestEnrichStreamLifts_HeuristicOff_OSMOn_NilRidesSafe(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()

	// Skip if osm_lifts has data — the nil-rides path short-circuits on empty
	// osm_lifts, which is the safe case this test is documenting.
	var hasOSM bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM osm_lifts LIMIT 1)").Scan(&hasOSM); err != nil {
		t.Skipf("cannot check osm_lifts: %v", err)
	}

	userID := liftTestUser(t, pool)
	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, activityID, userID, now, now.Add(time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id = $1", activityID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE id = $1", activityID)                        //nolint:errcheck
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, point_count)
		VALUES ($1, ST_GeomFromText('LINESTRING(12.0 47.0, 12.005 47.001)', 4326), 2)
	`, activityID); err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// heuristic=false, OSM=true via overrides.
	falsePtr := false
	truePtr := true
	settings := &model.EnrichmentSettings{
		EnableLiftDetection:          true,
		EnableLiftDetectionOSM:       true,
		EnableLiftDetectionHeuristic: false,
	}

	// Patch the activity so the activity-level override for heuristic is false,
	// confirming the opts resolver sees Heuristic=false.
	if _, err := pool.Exec(ctx, `
		UPDATE activities SET lift_detection_heuristic_override = $1,
		                      lift_detection_osm_override = $2
		WHERE id = $3
	`, &falsePtr, &truePtr, activityID); err != nil {
		t.Fatalf("set overrides: %v", err)
	}

	db := &database.DB{Pool: pool}
	stream := []EnrichmentStreamPoint{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("enrichStreamLiftsCore panicked with heuristic=false, OSM=true: %v", r)
		}
	}()

	if err := enrichStreamLiftsCore(ctx, db, userID, activityID, stream, false, false, settings); err != nil {
		t.Fatalf("enrichStreamLiftsCore: %v (want nil)", err)
	}

	// With empty osm_lifts, no candidates should have been inserted.
	if !hasOSM {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1
		`, activityID).Scan(&count); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 candidates (no OSM data), got %d", count)
		}
	}
}
