package database_test

// Phase-4 regression case: the drift-tolerance predicate used
// broad interval overlap instead of a per-boundary delta check. Any future
// candidate whose window overlapped a rejected window by any amount was
// suppressed, even when both boundaries had shifted far beyond ±5 s.
//
// These tests verify the corrected boundary-delta predicate:
//   |Δstarted_at| <= 5s AND |Δended_at| <= 5s

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestInsertHeuristicCandidateIfNotRejected_DriftWithinTolerance_Suppressed:
// A rejected window at [t, t+60s] and a new candidate at [t+3s, t+63s]
// (both boundaries shifted +3s, within ±5s) → the insert must be suppressed.
func TestInsertHeuristicCandidateIfNotRejected_DriftWithinTolerance_Suppressed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rejStart := now
	rejEnd := now.Add(60 * time.Second)

	rejectedID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, $4, 60, 50, 2, 200, 'high', false, 'heuristic', true)
	`, rejectedID, fx.activityID, rejStart, rejEnd); err != nil {
		t.Fatalf("insert rejected candidate: %v", err)
	}

	// New candidate: both boundaries shifted +3s (within ±5s tolerance).
	db := &database.DB{Pool: fx.pool}
	inserted, err := db.InsertHeuristicCandidateIfNotRejected(ctx, fx.userID, fx.activityID, database.HeuristicCandidateInput{
		StartedAt:  rejStart.Add(3 * time.Second),
		EndedAt:    rejEnd.Add(3 * time.Second),
		DurationS:  60,
		AscentM:    50,
		DescentM:   2.0,
		DistanceM:  200.0,
		Confidence: "high",
	})
	if err != nil {
		t.Fatalf("InsertHeuristicCandidateIfNotRejected: %v", err)
	}
	if inserted {
		t.Error("inserted = true, want false: candidate within ±5s on both boundaries must be suppressed")
	}
}

// TestInsertHeuristicCandidateIfNotRejected_PartialOverlapDistinct_NotSuppressed:
// A rejected window at [t, t+60s] and a new candidate at [t+30s, t+90s]
// (start shifted +30s, end shifted +30s — both far beyond ±5s) → the insert
// must proceed even though the windows partially overlap.
func TestInsertHeuristicCandidateIfNotRejected_PartialOverlapDistinct_NotSuppressed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rejStart := now
	rejEnd := now.Add(60 * time.Second)

	rejectedID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, $4, 60, 50, 2, 200, 'high', false, 'heuristic', true)
	`, rejectedID, fx.activityID, rejStart, rejEnd); err != nil {
		t.Fatalf("insert rejected candidate: %v", err)
	}

	// New candidate: both boundaries shifted +30s (> ±5s on both ends).
	db := &database.DB{Pool: fx.pool}
	inserted, err := db.InsertHeuristicCandidateIfNotRejected(ctx, fx.userID, fx.activityID, database.HeuristicCandidateInput{
		StartedAt:  rejStart.Add(30 * time.Second),
		EndedAt:    rejEnd.Add(30 * time.Second),
		DurationS:  60,
		AscentM:    50,
		DescentM:   2.0,
		DistanceM:  200.0,
		Confidence: "high",
	})
	if err != nil {
		t.Fatalf("InsertHeuristicCandidateIfNotRejected: %v", err)
	}
	if !inserted {
		t.Error("inserted = false, want true: partially overlapping but materially distinct candidate must be inserted")
	}
}

// TestInsertHeuristicCandidateIfNotRejected_BoundaryDriftExactly5s:
// Exactly ±5s on both boundaries is the boundary case — must still be
// suppressed (tolerance is inclusive, i.e. <=5s).
func TestInsertHeuristicCandidateIfNotRejected_BoundaryDriftExactly5s(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	rejStart := now
	rejEnd := now.Add(60 * time.Second)

	rejectedID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, $4, 60, 50, 2, 200, 'high', false, 'heuristic', true)
	`, rejectedID, fx.activityID, rejStart, rejEnd); err != nil {
		t.Fatalf("insert rejected candidate: %v", err)
	}

	// New candidate: both boundaries shifted exactly +5s.
	db := &database.DB{Pool: fx.pool}
	inserted, err := db.InsertHeuristicCandidateIfNotRejected(ctx, fx.userID, fx.activityID, database.HeuristicCandidateInput{
		StartedAt:  rejStart.Add(5 * time.Second),
		EndedAt:    rejEnd.Add(5 * time.Second),
		DurationS:  60,
		AscentM:    50,
		DescentM:   2.0,
		DistanceM:  200.0,
		Confidence: "high",
	})
	if err != nil {
		t.Fatalf("InsertHeuristicCandidateIfNotRejected: %v", err)
	}
	if inserted {
		t.Error("inserted = true, want false: exactly ±5s boundary drift must still be suppressed (inclusive tolerance)")
	}
}
