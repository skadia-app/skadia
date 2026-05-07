package database_test

// Phase-4 Adversarial-Critic finding F5: the rejected-row check and INSERT
// were separate statements, opening a window for a concurrent reject to
// land between them.
//
// This test fires Reject and Insert concurrently and asserts the rejected
// state always wins (either the insert was suppressed, or the row was
// rejected and the insert lost the race and inserted 0 rows).

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestInsertHeuristicCandidateIfNotRejected_RaceWithReject exercises the
// atomic-insert guarantee: concurrent calls to RejectLiftCandidate and
// InsertHeuristicCandidateIfNotRejected for the same activity must not
// produce a non-rejected row alongside a rejected one for the same window.
//
// The test seeds a non-rejected candidate, then races a reject against a
// re-insert of an equivalent candidate. After both goroutines complete, the
// table must not contain an un-rejected row that matches the window (the
// reject must win).
func TestInsertHeuristicCandidateIfNotRejected_RaceWithReject(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	candStart := now
	candEnd := now.Add(60 * time.Second)

	// Insert the initial candidate that will be rejected.
	existingID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, $4, 60, 50, 2, 200, 'high', false, 'heuristic')
	`, existingID, fx.activityID, candStart, candEnd); err != nil {
		t.Fatalf("insert initial candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}

	// Race many iterations to expose the window between check and insert.
	const iterations = 20
	for range iterations {
		// Reset: un-reject the existing row and delete any extra rows from
		// the previous iteration.
		if _, err := fx.pool.Exec(ctx,
			`UPDATE activity_lift_candidates SET rejected_by_user = false WHERE id = $1`,
			existingID,
		); err != nil {
			t.Fatalf("reset rejected flag: %v", err)
		}
		if _, err := fx.pool.Exec(ctx,
			`DELETE FROM activity_lift_candidates
			 WHERE activity_id = $1 AND id != $2`,
			fx.activityID, existingID,
		); err != nil {
			t.Fatalf("cleanup extra rows: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: reject the existing candidate.
		go func() {
			defer wg.Done()
			_ = db.RejectLiftCandidate(context.Background(), fx.userID, existingID, fx.activityID)
		}()

		// Goroutine 2: insert a candidate with the same window (±0s drift).
		go func() {
			defer wg.Done()
			_, _ = db.InsertHeuristicCandidateIfNotRejected(
				context.Background(), fx.userID, fx.activityID,
				database.HeuristicCandidateInput{
					StartedAt:  candStart,
					EndedAt:    candEnd,
					DurationS:  60,
					AscentM:    50,
					DescentM:   2.0,
					DistanceM:  200.0,
					Confidence: "high",
				},
			)
		}()

		wg.Wait()

		// Assert: no non-rejected row exists for the window. The rejected
		// state must win whenever both operations raced.
		var nonRejectedCount int
		if err := fx.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_lift_candidates
			WHERE activity_id = $1
			  AND started_at = $2
			  AND rejected_by_user = false
		`, fx.activityID, candStart).Scan(&nonRejectedCount); err != nil {
			t.Fatalf("count non-rejected rows: %v", err)
		}

		// If the existing row was rejected and a new row was also inserted
		// (non-rejected), that means the race was lost — the insert happened
		// after the reject check but before the reject landed.
		//
		// A non-rejected COUNT > 0 while rejected_by_user=true also exists is
		// the failure mode we're guarding against.
		var rejectedCount int
		if err := fx.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_lift_candidates
			WHERE activity_id = $1
			  AND started_at = $2
			  AND rejected_by_user = true
		`, fx.activityID, candStart).Scan(&rejectedCount); err != nil {
			t.Fatalf("count rejected rows: %v", err)
		}

		if rejectedCount > 0 && nonRejectedCount > 0 {
			t.Errorf("iteration: both a rejected row AND a non-rejected row exist for the same window — "+
				"race condition: insert won despite concurrent reject (rejected=%d, non_rejected=%d)",
				rejectedCount, nonRejectedCount)
		}
	}
}
