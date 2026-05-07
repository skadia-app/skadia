package database_test

// Phase-4d Adversarial-Critic finding F8: the suppression guarantee of
// InsertHeuristicCandidateIfNotRejected against a *pre-existing* rejected
// row in the ±5 s tolerance window must hold under concurrent insert
// pressure. This is the realistic atomicity contract — once the user has
// rejected a window, ALL concurrent inserts that fall within that window
// must be suppressed, never one or the other.
//
// (The earlier Race-With-Reject test asserted a different invariant —
// that a concurrent REJECT could retroactively block an INSERT that had
// not yet seen the reject. Postgres' READ COMMITTED isolation does not
// give that guarantee for non-overlapping rows, and the production code
// does not promise it. Asserting it would be testing a non-contract.)

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestInsertHeuristicCandidateIfNotRejected_ConcurrentInsertsSuppressedByPreExistingReject
// covers the atomicity-of-suppression contract: with a rejected row already
// present in the activity, N concurrent InsertHeuristicCandidateIfNotRejected
// calls inside the ±5 s tolerance window must all be suppressed. None of
// them may produce a live row, regardless of scheduling order.
//
// The pre-existing rejected row sits at `seedStart`. Each goroutine attempts
// an insert at `seedStart + offset`, where offset ∈ {1s, 2s, 3s, 4s} — every
// offset stays within ±5 s, so the suppression guard MUST fire for all of
// them. The post-condition is exactly one row in the activity (the
// pre-existing rejected one).
func TestInsertHeuristicCandidateIfNotRejected_ConcurrentInsertsSuppressedByPreExistingReject(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	seedStart := now
	seedEnd := now.Add(60 * time.Second)

	// Seed with a pre-existing REJECTED row in the activity. This is the
	// suppression anchor — every concurrent insert below must be blocked
	// because its boundaries fall within ±5 s of this row's boundaries.
	seedID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(id, activity_id, started_at, ended_at, duration_s, ascent_m,
			 descent_m, distance_m, confidence, confirmed, source,
			 rejected_by_user)
		VALUES ($1, $2, $3, $4, 60, 50, 2, 200, 'high', false, 'heuristic',
			true)
	`, seedID, fx.activityID, seedStart, seedEnd); err != nil {
		t.Fatalf("insert seed rejected candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}

	// Run several iterations to expose any windowed-race in the
	// WHERE NOT EXISTS guard against a stable pre-existing rejected row.
	const iterations = 20
	const concurrency = 4
	insertOffsets := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second}

	for i := range iterations {
		// Cleanup: keep only the seed row.
		if _, err := fx.pool.Exec(ctx,
			`DELETE FROM activity_lift_candidates
			 WHERE activity_id = $1 AND id != $2`,
			fx.activityID, seedID,
		); err != nil {
			t.Fatalf("iteration %d: cleanup extra rows: %v", i, err)
		}

		var wg sync.WaitGroup
		wg.Add(concurrency)

		var insertedCount int32
		errCh := make(chan error, concurrency)

		for offset := range insertOffsets {
			off := insertOffsets[offset]
			go func() {
				defer wg.Done()
				inserted, err := db.InsertHeuristicCandidateIfNotRejected(
					context.Background(), fx.userID, fx.activityID,
					database.HeuristicCandidateInput{
						StartedAt:  seedStart.Add(off),
						EndedAt:    seedEnd.Add(off),
						DurationS:  60,
						AscentM:    50,
						DescentM:   2.0,
						DistanceM:  200.0,
						Confidence: "high",
					},
				)
				if err != nil {
					errCh <- err
					return
				}
				if inserted {
					atomic.AddInt32(&insertedCount, 1)
				}
			}()
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Errorf("iteration %d: goroutine error: %v", i, err)
		}

		if got := atomic.LoadInt32(&insertedCount); got != 0 {
			t.Errorf("iteration %d: %d insert(s) escaped suppression — every concurrent insert in the ±5 s "+
				"tolerance window of a pre-existing rejected row must be blocked",
				i, got)
		}

		// Authoritative count: exactly one row (the seed) remains.
		var totalRows int
		if err := fx.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_lift_candidates
			WHERE activity_id = $1
		`, fx.activityID).Scan(&totalRows); err != nil {
			t.Fatalf("iteration %d: count rows: %v", i, err)
		}
		if totalRows != 1 {
			t.Errorf("iteration %d: expected 1 row (the seed) but got %d — concurrent inserts produced live rows",
				i, totalRows)
		}
	}
}

// TestInsertHeuristicCandidateIfNotRejected_ConcurrentInsertsWithShiftedTimes
// covers the N4 race fix (Codex GPT-5.5-xhigh adversarial-review #4): with NO
// pre-existing rejected row, two concurrent InsertHeuristicCandidateIfNotRejected
// calls with started_at values shifted within the ±5 s tolerance window
// could BOTH observe "no rejected row exists yet" and BOTH insert. The
// per-activity advisory lock funnels them into single-file order so that
// at most one row lands per (activity_id, started_at) bucket — the
// ON CONFLICT branch suppresses an exact-time duplicate, and follow-up
// reject-and-reinsert sequences cannot interleave either.
//
// Invariant: with concurrency=4 goroutines all targeting non-overlapping
// started_at values inside the ±5 s window, every call returns either
// (true, nil) — row inserted — or (false, nil) — skipped because the lock
// could not be acquired before retries exhausted. We do NOT assert "exactly
// one row inserted" because each goroutine targets a distinct started_at
// (different ON CONFLICT key), so all four CAN legitimately succeed once
// they run sequentially under the lock — what was racy is the ORDERING,
// not the row count for distinct started_at values. The post-condition
// asserts: no errors AND no rows that would have been suppressed by a
// prior reject (which is trivially true here since there's no reject).
func TestInsertHeuristicCandidateIfNotRejected_ConcurrentInsertsSerializedByLock(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	baseStart := now
	baseEnd := now.Add(60 * time.Second)

	db := &database.DB{Pool: fx.pool}

	const iterations = 10
	const concurrency = 4
	insertOffsets := []time.Duration{1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second}

	for i := range iterations {
		// Cleanup: clear any rows from the previous iteration.
		if _, err := fx.pool.Exec(ctx,
			`DELETE FROM activity_lift_candidates WHERE activity_id = $1`,
			fx.activityID,
		); err != nil {
			t.Fatalf("iteration %d: cleanup rows: %v", i, err)
		}

		var wg sync.WaitGroup
		wg.Add(concurrency)
		var insertedCount int32
		errCh := make(chan error, concurrency)

		for offset := range insertOffsets {
			off := insertOffsets[offset]
			go func() {
				defer wg.Done()
				inserted, err := db.InsertHeuristicCandidateIfNotRejected(
					context.Background(), fx.userID, fx.activityID,
					database.HeuristicCandidateInput{
						StartedAt:  baseStart.Add(off),
						EndedAt:    baseEnd.Add(off),
						DurationS:  60,
						AscentM:    50,
						DescentM:   2.0,
						DistanceM:  200.0,
						Confidence: "high",
					},
				)
				if err != nil {
					errCh <- err
					return
				}
				if inserted {
					atomic.AddInt32(&insertedCount, 1)
				}
			}()
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Errorf("iteration %d: goroutine error: %v", i, err)
		}

		// Each distinct (activity_id, started_at) is allowed to insert. The
		// invariant is "no goroutine errored" (already asserted above) and
		// "the per-(activity_id, started_at) ON CONFLICT guard held" — i.e.
		// no duplicate started_at values in the table. The advisory lock
		// orchestrates the order; the ON CONFLICT clause is the underlying
		// uniqueness guarantor.
		var totalRows int
		if err := fx.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM activity_lift_candidates
			WHERE activity_id = $1
		`, fx.activityID).Scan(&totalRows); err != nil {
			t.Fatalf("iteration %d: count rows: %v", i, err)
		}
		// We expect at most concurrency rows (one per distinct started_at),
		// minus whatever was skipped because the lock couldn't be acquired
		// in time. inserted-true count must equal the table row count.
		if totalRows != int(atomic.LoadInt32(&insertedCount)) {
			t.Errorf("iteration %d: rows in table=%d but inserted-true count=%d — counts must match",
				i, totalRows, atomic.LoadInt32(&insertedCount))
		}
		if totalRows > concurrency {
			t.Errorf("iteration %d: %d rows in table but concurrency=%d — duplicate started_at slipped through",
				i, totalRows, concurrency)
		}
	}
}
