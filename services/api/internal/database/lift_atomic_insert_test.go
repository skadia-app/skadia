package database_test

// Phase-4 Adversarial-Critic finding F5: the rejected-row check and INSERT
// were separate statements, opening a window for a concurrent reject to
// land between them.
//
// Race scenario: a heuristic candidate already exists in the table. A
// concurrent insert uses a started_at that is within ±5 s of the existing
// row (so it matches the boundary-delta suppression check) but different
// enough to avoid the UNIQUE constraint — this gives InsertHeuristic…
// something to actually insert rather than hitting ON CONFLICT DO NOTHING.
// If Reject wins, the new insert is suppressed by the WHERE NOT EXISTS
// guard. If Insert wins first, the new row is live while the original gets
// rejected; that combination (a rejected row + a non-rejected near-window
// row) is the invariant violation the test catches.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestInsertHeuristicCandidateIfNotRejected_RaceWithReject exercises the
// atomic-insert guarantee: a concurrent RejectLiftCandidate and
// InsertHeuristicCandidateIfNotRejected for the same activity must not
// produce a live (non-rejected) row for a window that has already been
// rejected.
//
// The candidate used for InsertHeuristic… has a started_at shifted by 3 s
// from the existing row — within the ±5 s tolerance window, so the
// WHERE NOT EXISTS guard would suppress it when a rejected row exists —
// but different enough to avoid the UNIQUE (activity_id, started_at)
// conflict that would make the INSERT a silent no-op regardless of the
// rejection state.
func TestInsertHeuristicCandidateIfNotRejected_RaceWithReject(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	existingStart := now
	existingEnd := now.Add(60 * time.Second)

	// The insert candidate is shifted 3 s inside the ±5 s suppression window
	// so the WHERE NOT EXISTS guard fires when a rejected row is present, but
	// the started_at differs enough to not hit the UNIQUE constraint.
	insertStart := now.Add(3 * time.Second)
	insertEnd := existingEnd.Add(3 * time.Second)

	// Insert the initial candidate that will be rejected in each iteration.
	existingID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, $4, 60, 50, 2, 200, 'high', false, 'heuristic')
	`, existingID, fx.activityID, existingStart, existingEnd); err != nil {
		t.Fatalf("insert initial candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}

	// Race many iterations to expose the window between check and insert.
	const iterations = 20
	for i := range iterations {
		// Reset: un-reject the existing row and delete any extra rows from
		// the previous iteration.
		if _, err := fx.pool.Exec(ctx,
			`UPDATE activity_lift_candidates SET rejected_by_user = false WHERE id = $1`,
			existingID,
		); err != nil {
			t.Fatalf("iteration %d: reset rejected flag: %v", i, err)
		}
		if _, err := fx.pool.Exec(ctx,
			`DELETE FROM activity_lift_candidates
			 WHERE activity_id = $1 AND id != $2`,
			fx.activityID, existingID,
		); err != nil {
			t.Fatalf("iteration %d: cleanup extra rows: %v", i, err)
		}

		var wg sync.WaitGroup
		wg.Add(2)

		errCh := make(chan error, 2)

		// Goroutine 1: reject the existing candidate.
		go func() {
			defer wg.Done()
			if err := db.RejectLiftCandidate(context.Background(), fx.userID, existingID, fx.activityID); err != nil {
				errCh <- err
			}
		}()

		// Goroutine 2: insert a candidate within the ±5 s suppression window
		// of the existing row. When the reject wins the race, the WHERE NOT
		// EXISTS guard must suppress this insert.
		go func() {
			defer wg.Done()
			if _, err := db.InsertHeuristicCandidateIfNotRejected(
				context.Background(), fx.userID, fx.activityID,
				database.HeuristicCandidateInput{
					StartedAt:  insertStart,
					EndedAt:    insertEnd,
					DurationS:  60,
					AscentM:    50,
					DescentM:   2.0,
					DistanceM:  200.0,
					Confidence: "high",
				},
			); err != nil {
				errCh <- err
			}
		}()

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Errorf("iteration %d: goroutine error: %v", i, err)
		}

		// Count rows per rejection state for the activity window.
		var rejectedCount, nonRejectedCount int
		if err := fx.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_lift_candidates
			WHERE activity_id = $1
			  AND rejected_by_user = true
		`, fx.activityID).Scan(&rejectedCount); err != nil {
			t.Fatalf("iteration %d: count rejected rows: %v", i, err)
		}
		if err := fx.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_lift_candidates
			WHERE activity_id = $1
			  AND rejected_by_user = false
		`, fx.activityID).Scan(&nonRejectedCount); err != nil {
			t.Fatalf("iteration %d: count non-rejected rows: %v", i, err)
		}

		// Invariant: if any row was rejected, no non-rejected row within the
		// same suppression window may exist. The reject must win whenever
		// both goroutines raced; finding both states simultaneously means the
		// WHERE NOT EXISTS guard failed to suppress the insert.
		if rejectedCount > 0 && nonRejectedCount > 0 {
			t.Errorf("iteration %d: both a rejected row AND a non-rejected row exist for the same activity — "+
				"race condition: insert escaped the WHERE NOT EXISTS guard (rejected=%d, non_rejected=%d)",
				i, rejectedCount, nonRejectedCount)
		}
	}
}
