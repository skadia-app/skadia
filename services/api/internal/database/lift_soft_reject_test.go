package database_test

// Phase 4d.3 — soft-reject filter + drift-tolerance tests.
//
// TestRecomputeLiftStats_RejectedCandidatesExcluded (4d.3.1): 5 heuristic
// candidates, 2 rejected → recompute counts only the 3 active ones.
//
// TestRejectLiftCandidate_DriftTolerance (4d.3.2): rejected at T0,
// re-detection would insert at T0+3s (within ±5s overlap tolerance) →
// no new row inserted.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestRecomputeLiftStats_RejectedCandidatesExcluded verifies that
// recomputeLiftStats does not count candidates with rejected_by_user=true,
// even when they are marked confirmed=true.
func TestRecomputeLiftStats_RejectedCandidatesExcluded(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// Insert 5 heuristic candidates, confirmed=true.
	// Candidates 0 and 1 will be rejected; 2–4 stay active.
	var candidateIDs [5]string
	for i := range 5 {
		cID := uuid.NewString()
		candidateIDs[i] = cID
		start := now.Add(time.Duration(i*200) * time.Second)
		end := start.Add(180 * time.Second)
		if _, err := fx.pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(id, activity_id, started_at, ended_at, duration_s, ascent_m,
				 descent_m, distance_m, confidence, confirmed, source)
			VALUES ($1, $2, $3, $4, 180, 100, 5, 400, 'high', true, 'heuristic')
		`, cID, fx.activityID, start, end); err != nil {
			t.Fatalf("insert candidate %d: %v", i, err)
		}
	}

	// Reject candidates 0 and 1.
	if _, err := fx.pool.Exec(ctx, `
		UPDATE activity_lift_candidates SET rejected_by_user = true
		WHERE id = ANY($1)
	`, []string{candidateIDs[0], candidateIDs[1]}); err != nil {
		t.Fatalf("reject candidates: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var liftCount int
	var liftDur, liftAsc, liftDist float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT lift_count, lift_duration_s, lift_ascent_m, lift_distance_m
		FROM activities WHERE id = $1
	`, fx.activityID).Scan(&liftCount, &liftDur, &liftAsc, &liftDist); err != nil {
		t.Fatalf("query stats: %v", err)
	}

	// Only the 3 non-rejected candidates should count.
	if liftCount != 3 {
		t.Errorf("lift_count = %d, want 3 (2 rejected must be excluded)", liftCount)
	}
	if liftDur != 3*180 {
		t.Errorf("lift_duration_s = %.0f, want %d", liftDur, 3*180)
	}
	if liftAsc != 3*100 {
		t.Errorf("lift_ascent_m = %.0f, want %d", liftAsc, 3*100)
	}
	if liftDist != 3*400 {
		t.Errorf("lift_distance_m = %.0f, want %d", liftDist, 3*400)
	}
}

// TestRejectLiftCandidate_DriftTolerance verifies that when a candidate is
// rejected at T0 and the detector would re-insert at T0+3s (within ±5s
// tolerance), insertHeuristicCandidateWithRejectCheck skips the insert.
//
// This test calls the exported InsertHeuristicCandidateIfNotRejected helper
// (introduced in Phase 4d) rather than calling enrichStreamLifts directly
// (too many dependencies for a unit-level test).
func TestRejectLiftCandidate_DriftTolerance(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	cStart := now
	cEnd := now.Add(5 * time.Minute)

	// Insert a rejected candidate at T0.
	rejectedID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, $4, 300, 200, 5, 800, 'high', false, 'heuristic', true)
	`, rejectedID, fx.activityID, cStart, cEnd); err != nil {
		t.Fatalf("insert rejected candidate: %v", err)
	}

	// Simulate re-detection: started_at = T0+3s (within ±5s tolerance).
	driftStart := cStart.Add(3 * time.Second)
	driftEnd := cEnd.Add(3 * time.Second)

	db := &database.DB{Pool: fx.pool}
	inserted, err := db.InsertHeuristicCandidateIfNotRejected(ctx, fx.userID, fx.activityID, database.HeuristicCandidateInput{
		StartedAt:  driftStart,
		EndedAt:    driftEnd,
		DurationS:  300,
		AscentM:    200,
		DescentM:   5.0,
		DistanceM:  800.0,
		Confidence: "high",
	})
	if err != nil {
		t.Fatalf("InsertHeuristicCandidateIfNotRejected: %v", err)
	}
	if inserted {
		t.Error("inserted = true, want false (rejected row within ±5s must block re-insert)")
	}

	// Verify no new row was added.
	var count int
	if err := fx.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1",
		fx.activityID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("candidate count = %d, want 1 (re-detection must not add row)", count)
	}
}

