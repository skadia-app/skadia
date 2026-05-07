package enrichment

// Phase 4d.2 + 4d.3 tests for soft-reject persistence across re-enrichment.
//
// TestRejectLiftCandidate_SoftDeletePersistsAcrossReEnrichment: verifies that
// a rejected candidate survives a full enrichStreamLifts re-run and is not
// re-counted in lift stats.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestRejectLiftCandidate_SoftDeletePersistsAcrossReEnrichment sets up one
// activity with one heuristic candidate, rejects it via RejectLiftCandidate,
// then runs a full enrichStreamLifts pass and asserts:
//  1. The rejected candidate still exists in the DB (rejected_by_user = true).
//  2. It is NOT counted in activities.lift_count (aggregate filter works).
func TestRejectLiftCandidate_SoftDeletePersistsAcrossReEnrichment(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()

	userID := liftTestUser(t, pool)
	// Use a ski activity (lift-relevant sport) so enrichStreamLifts proceeds.
	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'skiing', $3, $4, $5)
	`, activityID, userID, now, now.Add(2*time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id = $1", activityID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM segment_efforts WHERE activity_id = $1", activityID)          //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_tracks WHERE activity_id = $1", activityID)          //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_streams WHERE activity_id = $1", activityID)         //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE id = $1", activityID)                        //nolint:errcheck
	})

	// Insert a minimal track so the track-exists check passes.
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, point_count)
		VALUES ($1, ST_GeomFromText('LINESTRING(11.0 47.0, 11.01 47.01)', 4326), 2)
	`, activityID); err != nil {
		t.Fatalf("insert track: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_streams (activity_id, time, lat, lng)
		VALUES ($1, $2, 47.0, 11.0), ($1, $3, 47.01, 11.01)
	`, activityID, now, now.Add(30*time.Second)); err != nil {
		t.Fatalf("insert streams: %v", err)
	}

	// Insert a heuristic candidate directly (simulating a prior enrichment run).
	candidateID := uuid.New().String()
	cStart := now.Add(-10 * time.Minute) // clearly distinct from any detector output
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, $4, 300, 200, 10, 800, 'high', false, 'heuristic')
	`, candidateID, activityID, cStart, cStart.Add(5*time.Minute)); err != nil {
		t.Fatalf("insert candidate: %v", err)
	}

	// Soft-reject it via the DB method under test.
	db := &database.DB{Pool: pool}
	if err := db.RejectLiftCandidate(ctx, candidateID, activityID); err != nil {
		t.Fatalf("RejectLiftCandidate: %v", err)
	}

	// Verify it's marked rejected (not deleted).
	var rejectedByUser bool
	if err := pool.QueryRow(ctx,
		"SELECT rejected_by_user FROM activity_lift_candidates WHERE id = $1",
		candidateID,
	).Scan(&rejectedByUser); err != nil {
		t.Fatalf("query rejected_by_user after reject: %v", err)
	}
	if !rejectedByUser {
		t.Fatal("rejected_by_user = false after RejectLiftCandidate — soft-reject did not write")
	}

	// Run enrichStreamLifts (equivalent of a re-enrichment pass).
	// In the no-OSM environment the heuristic path runs but finds nothing
	// (synthetic 2-point track with no elevation signal → no detections).
	// In an OSM environment Method-A may insert new candidates but must
	// not touch the rejected one.
	lat0, lng0 := 47.0, 11.0
	lat1, lng1 := 47.01, 11.01
	stream := []EnrichmentStreamPoint{
		{Time: now, Lat: &lat0, Lng: &lng0},
		{Time: now.Add(30 * time.Second), Lat: &lat1, Lng: &lng1},
	}
	if err := enrichStreamLifts(ctx, db, userID, activityID, stream); err != nil {
		t.Fatalf("enrichStreamLifts: %v", err)
	}

	// The rejected candidate must still exist with rejected_by_user = true.
	if err := pool.QueryRow(ctx,
		"SELECT rejected_by_user FROM activity_lift_candidates WHERE id = $1",
		candidateID,
	).Scan(&rejectedByUser); err != nil {
		t.Fatalf("query rejected_by_user after re-enrichment: candidate was hard-deleted (Phase 4d regression: %v)", err)
	}
	if !rejectedByUser {
		t.Error("rejected_by_user = false after re-enrichment — soft-reject flag was cleared")
	}

	// The rejected candidate must NOT appear in lift_count.
	var liftCount int
	if err := pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query lift_count: %v", err)
	}
	// The rejected candidate is confirmed=false, so even before 4d it would not
	// count. To make the filter assertion meaningful we confirm it in-place and
	// then re-run recomputeLiftStats.
	if _, err := pool.Exec(ctx,
		"UPDATE activity_lift_candidates SET confirmed = true WHERE id = $1", candidateID,
	); err != nil {
		t.Fatalf("set confirmed=true for filter assertion: %v", err)
	}
	if err := db.RecomputeLiftStats(ctx, activityID); err != nil {
		t.Fatalf("recompute after confirm: %v", err)
	}
	if err := pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query lift_count after confirm: %v", err)
	}
	// rejected_by_user=true must override confirmed=true in the aggregate filter.
	if liftCount != 0 {
		t.Errorf("lift_count = %d with rejected+confirmed candidate, want 0 (rejected must not count)", liftCount)
	}
}
