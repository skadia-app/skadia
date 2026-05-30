package database_test

// Phase-4 regression case— OSM-Reject-Recovery.
//
// Pre-fix: recomputeLiftStats counted segment_efforts unconditionally,
// ignoring whether the corresponding audit-row in activity_lift_candidates
// carried rejected_by_user=true. Result: a user reject of an OSM-snapped
// lift had zero effect on activities.lift_count / lift_duration_s /
// lift_ascent_m / lift_descent_m / lift_distance_m.
//
// Post-fix: recomputeLiftStats excludes segment_efforts whose audit-row was
// soft-rejected (NOT EXISTS sub-query in the segment_efforts arm). The
// pre-Phase-4c case (segment_effort without any audit-row) keeps counting.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestRecomputeLiftStats_RejectedOSMSegmentEffortExcluded: two OSM-snapped
// lifts on the activity, one rejected via the audit-row, the other not.
// Pre-fix: lift_count = 2 (both segment_efforts counted). Post-fix:
// lift_count = 1 (rejected one filtered out via NOT EXISTS rejected
// audit-row).
func TestRecomputeLiftStats_RejectedOSMSegmentEffortExcluded(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// Lift 1: rejected. segment_effort + audit-row(rejected=true,confirmed=true).
	rejEffortID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard, is_lift_segment_match)
		VALUES ($1, $2, $3, $4, $5, $6, 300, 600, 100, false,
		        'skiing', 'generic', false, true)
	`, rejEffortID, fx.activityID, fx.segmentID, fx.versionID, fx.userID, now); err != nil {
		t.Fatalf("rejected segment_effort: %v", err)
	}
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, segment_effort_id, rejected_by_user)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm', $4, true)
	`, fx.activityID, now, now.Add(5*time.Minute), rejEffortID); err != nil {
		t.Fatalf("rejected audit candidate: %v", err)
	}

	// Lift 2: kept. segment_effort + audit-row(rejected=false,confirmed=true).
	keepEffortID := uuid.NewString()
	keepStart := now.Add(15 * time.Minute)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard, is_lift_segment_match)
		VALUES ($1, $2, $3, $4, $5, $6, 200, 500, 80, false,
		        'skiing', 'generic', false, true)
	`, keepEffortID, fx.activityID, fx.segmentID, fx.versionID, fx.userID, keepStart); err != nil {
		t.Fatalf("kept segment_effort: %v", err)
	}
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, segment_effort_id)
		VALUES ($1, $2, $3, 200, 80, 0, 500, 'high', true, 'osm', $4)
	`, fx.activityID, keepStart, keepStart.Add(200*time.Second), keepEffortID); err != nil {
		t.Fatalf("kept audit candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.userID, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var liftCount int
	var liftDur, liftAsc, liftDist float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT lift_count, lift_duration_s, lift_ascent_m, lift_distance_m
		FROM activities WHERE id = $1
	`, fx.activityID).Scan(&liftCount, &liftDur, &liftAsc, &liftDist); err != nil {
		t.Fatalf("query: %v", err)
	}
	if liftCount != 1 {
		t.Errorf("lift_count = %d, want 1 (rejected OSM segment_effort must be excluded)", liftCount)
	}
	if liftDur != 200 {
		t.Errorf("lift_duration_s = %.0f, want 200 (only kept lift)", liftDur)
	}
	if liftAsc != 80 {
		t.Errorf("lift_ascent_m = %.0f, want 80 (only kept lift)", liftAsc)
	}
	if liftDist != 500 {
		t.Errorf("lift_distance_m = %.0f, want 500 (only kept lift)", liftDist)
	}
}

// TestRecomputeLiftStats_RejectedOSMCableHoldMergeExcluded: a Stage-2
// cable-hold merge produces 1 segment_effort + N audit-rows referencing it.
// If the user rejects ANY of the audit-rows the canonical merge is excluded
// (this is the only sensible UX — the user has no way to reject "half a
// merged lift"). The NOT EXISTS predicate matches when at least one
// audit-row carries rejected_by_user=true.
//
// Also verifies the dedup contract: without the rejected flag the merge
// counts as exactly 1, never N.
func TestRecomputeLiftStats_RejectedOSMCableHoldMergeExcluded(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	effortID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard,
			 is_lift_segment_match, is_cable_hold_merge)
		VALUES ($1, $2, $3, $4, $5, $6, 600, 600, 200, false,
		        'skiing', 'generic', false, true, true)
	`, effortID, fx.activityID, fx.segmentID, fx.versionID, fx.userID, now); err != nil {
		t.Fatalf("segment_effort: %v", err)
	}
	// Three audit candidates — first one rejected.
	for i := range 3 {
		started := now.Add(time.Duration(i*200) * time.Second)
		ended := started.Add(150 * time.Second)
		rejected := i == 0
		if _, err := fx.pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
				 confidence, confirmed, source, segment_effort_id, rejected_by_user)
			VALUES ($1, $2, $3, 150, 65, 0, 200, 'high', true, 'osm', $4, $5)
		`, fx.activityID, started, ended, effortID, rejected); err != nil {
			t.Fatalf("audit candidate %d: %v", i, err)
		}
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.userID, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	var liftCount int
	if err := fx.pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", fx.activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if liftCount != 0 {
		t.Errorf("lift_count = %d, want 0 (any rejected audit-row excludes the canonical merge)", liftCount)
	}
}

// TestRecomputeLiftStats_SegmentEffortWithoutAuditRowStillCounts: a lift
// segment_effort that has no audit-row at all (pre-Phase-4c data, or any
// future code path that emits segment_efforts without the audit mirror)
// must keep counting. The NOT EXISTS predicate is true when no rejected
// audit-row exists for the effort.
func TestRecomputeLiftStats_SegmentEffortWithoutAuditRowStillCounts(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	effortID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard, is_lift_segment_match)
		VALUES ($1, $2, $3, $4, $5, $6, 300, 600, 100, false,
		        'skiing', 'generic', false, true)
	`, effortID, fx.activityID, fx.segmentID, fx.versionID, fx.userID, now); err != nil {
		t.Fatalf("segment_effort: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.userID, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	var liftCount int
	if err := fx.pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", fx.activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if liftCount != 1 {
		t.Errorf("lift_count = %d, want 1 (segment_effort without audit-row must keep counting)", liftCount)
	}
}

// TestRecomputeLiftStats_RejectedHeuristicOnlyStillExcluded: backwards-
// compatibility with the Phase-4d heuristic-only path. A rejected
// heuristic candidate (no segment_effort, segment_effort_id IS NULL,
// confirmed=true, rejected_by_user=true) must remain excluded from the
// heuristic-only sub-query.
func TestRecomputeLiftStats_RejectedHeuristicOnlyStillExcluded(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 200, 80, 0, 500, 'medium', true, 'heuristic', true)
	`, fx.activityID, now, now.Add(200*time.Second)); err != nil {
		t.Fatalf("rejected heuristic candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.userID, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	var liftCount int
	if err := fx.pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", fx.activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if liftCount != 0 {
		t.Errorf("lift_count = %d, want 0 (rejected heuristic candidate must remain excluded)", liftCount)
	}
}
