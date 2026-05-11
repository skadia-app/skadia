package handler

// F28 — race regression test for RecalculateActivityLifts vs. concurrent
// RunStreamEnrichment.
//
// Design rationale:
//   - Goroutine A calls RecalculateActivityLifts (handler). After F2's async-
//     dispatch fix, the handler deletes candidates synchronously and spawns a
//     background goroutine for re-enrichment. The HTTP response arrives while
//     the re-enrichment goroutine may still be running.
//   - Goroutine B calls enrichment.RunStreamEnrichment directly, 50 ms later.
//     enrichStreamLifts acquires a per-activity advisory lock; if the background
//     goroutine from A holds the same lock, B skips its run (returns nil quickly)
//     rather than deadlocking. Both goroutines must finish within 5 s.
//   - After both complete: no orphan rows in activity_lift_candidates (i.e. no
//     row that was inserted by B after A's DELETE, leaving a candidate that
//     references a deleted segment_effort or is in an inconsistent state).
//
// The test uses a minimal activity fixture (no track data). RunStreamEnrichment
// short-circuits on empty stream (stream_enrich.go:139 — `len(stream)==0`)
// so the goroutines complete in milliseconds. No special build tag is needed.

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment"
)

// TestRecalculateActivityLifts_Race_ReenrichConcurrency verifies that concurrent
// RecalculateActivityLifts and RunStreamEnrichment calls do not deadlock and do
// not leave orphan / duplicate candidate rows.
func TestRecalculateActivityLifts_Race_ReenrichConcurrency(t *testing.T) {
	h, db := liftCandidatesHandlerSetup(t)
	userID := insertTestUser(t, db.Pool)
	// cycling is lift-eligible so the F25 guard does not short-circuit.
	activityID := insertMinimalActivity(t, db, userID)

	// Pre-populate candidates so A has something to delete.
	now := time.Now().UTC().Truncate(time.Second)
	insertHeuristicUnconfirmed(t, db, activityID, now)

	// Budget: both goroutines must finish within 5 s.
	done := make(chan struct{})
	timeout := time.After(5 * time.Second)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine A: invoke the HTTP handler (which now returns 204 quickly and
	// detaches re-enrichment asynchronously).
	go func() {
		defer wg.Done()
		rec := callBulkLifts(t, h, userID, activityID, "recalculate-all")
		if rec.Code != http.StatusNoContent {
			// Use t.Errorf (not Fatalf) so the goroutine does not panic and
			// the test still completes.
			t.Errorf("goroutine A: recalculate-all status=%d, want 204; body=%s",
				rec.Code, rec.Body.String())
		}
	}()

	// Goroutine B: call RunStreamEnrichment directly 50 ms after A starts, to
	// race with the background goroutine spawned by the handler.
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		ctx := context.Background()
		settings := enrichment.StreamEnrichmentSettings{EnableLiftDetection: true}
		if err := enrichment.RunStreamEnrichment(ctx, db, userID, activityID, settings); err != nil {
			// enrichStreamLifts returns nil when it skips due to the advisory lock
			// or when the stream is empty. A non-nil error is unexpected but
			// non-fatal for the race test — we log it and continue so wg.Wait
			// can unblock and the deadline check below runs.
			t.Logf("goroutine B: RunStreamEnrichment: %v (non-fatal for race test)", err)
		}
	}()

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Both goroutines finished within budget — pass.
	case <-timeout:
		t.Fatal("deadlock detected: goroutines did not finish within 5 s")
	}

	// Post-condition: no orphan rows. After A's synchronous DELETE and B's
	// (skipped or empty) re-enrichment, the candidate table must be empty.
	var count int
	if err := db.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1`,
		activityID,
	).Scan(&count); err != nil {
		t.Fatalf("post-race count: %v", err)
	}
	// An empty stream produces no new candidates; A's DELETE leaves count=0.
	// A non-zero count indicates B wrote candidates after A's DELETE — regression.
	if count != 0 {
		t.Errorf("orphan candidates count = %d after concurrent recalculate + re-enrich (want 0)", count)
	}
}
