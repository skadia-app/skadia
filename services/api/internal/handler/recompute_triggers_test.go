//go:build integration

package handler

// Integration test pinning the readiness-trigger contract for RecomputeAllTSS
// (R1-#11): after a successful async TSS recompute the handler must call
// AnalyticsInvalidator.PublishAnalyticsInvalidate with the authenticated user's
// ID.
//
// The handler launches the recompute work via shutdown.Go (background goroutine).
// The test uses a channel-based FakeInvalidator (local to this file) to detect
// the invalidate call without polling. shutdown.Reset() is registered as a
// t.Cleanup to drain the WaitGroup so no goroutines leak between tests.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/pmc"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// fakeInvalidatorWithDone is a thread-safe AnalyticsInvalidator that closes
// its Done channel on the first PublishAnalyticsInvalidate call. Local to this
// file so shared helpers in testhelpers_test.go are not modified.
type fakeInvalidatorWithDone struct {
	mu    sync.Mutex
	calls []string
	once  sync.Once
	done  chan struct{}
}

func newFakeInvalidatorWithDone() *fakeInvalidatorWithDone {
	return &fakeInvalidatorWithDone{done: make(chan struct{})}
}

func (f *fakeInvalidatorWithDone) PublishAnalyticsInvalidate(userID string) error {
	f.mu.Lock()
	f.calls = append(f.calls, userID)
	f.mu.Unlock()
	f.once.Do(func() { close(f.done) })
	return nil
}

func (f *fakeInvalidatorWithDone) callsCopy() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// waitDone blocks until the done channel is closed or the deadline is reached.
// Returns true if the channel was closed before the deadline.
func (f *fakeInvalidatorWithDone) waitDone(deadline time.Duration) bool {
	select {
	case <-f.done:
		return true
	case <-time.After(deadline):
		return false
	}
}

// TestRecomputeAllTSS_Triggers verifies that the POST /api/v1/admin/recompute-tss
// handler fires PublishAnalyticsInvalidate after the background goroutine
// completes successfully. Uses 0 activities — RecomputeAllTSS returns nil
// immediately (RecalculatePMC exits early when there are no activities) and
// the trigger must still fire.
func TestRecomputeAllTSS_Triggers_AnalyticsInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	// Clean up background_jobs and notifications rows created during the test.
	// background_jobs.user_id is plain TEXT — no FK cascade from users deletion.
	t.Cleanup(func() {
		ctx := context.Background()
		pool.Exec(ctx, "DELETE FROM notifications WHERE user_id = $1", uid)    //nolint:errcheck
		pool.Exec(ctx, "DELETE FROM background_jobs WHERE user_id = $1", uid)  //nolint:errcheck
	})

	// Drain all shutdown.Go goroutines at test end so no goroutines leak.
	t.Cleanup(shutdown.Reset)

	fake := newFakeInvalidatorWithDone()
	h := &Handler{
		DB:                   &database.DB{Pool: pool},
		PMC:                  &pmc.Service{DB: &database.DB{Pool: pool}},
		AnalyticsInvalidator: fake,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/recompute-tss", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, true))
	rec := httptest.NewRecorder()
	h.RecomputeAllTSS(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("RecomputeAllTSS: code = %d body = %s, want 202", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "started" {
		t.Fatalf("response status = %q, want \"started\"", resp["status"])
	}

	// Wait for the background goroutine to fire AnalyticsInvalidate (max 30s).
	if !fake.waitDone(30 * time.Second) {
		t.Fatal("PublishAnalyticsInvalidate was not called within 30s — trigger not fired")
	}

	calls := fake.callsCopy()
	if len(calls) != 1 {
		t.Fatalf("FakeInvalidator.calls = %d, want 1", len(calls))
	}
	if calls[0] != uid {
		t.Errorf("FakeInvalidator.calls[0] = %q, want %q", calls[0], uid)
	}
}

// TestRecomputeAllTSS_AlreadyRunning_NoInvalidate verifies that when a TSS
// recompute job is already running (InsertBackgroundJobGuarded returns
// already_running), the handler returns 200 and does NOT call
// PublishAnalyticsInvalidate (no goroutine is launched).
func TestRecomputeAllTSS_AlreadyRunning_NoInvalidate(t *testing.T) {
	pool := fusionTestPool(t)
	uid := insertTestUser(t, pool)

	ctx := context.Background()
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM notifications WHERE user_id = $1", uid)   //nolint:errcheck
		pool.Exec(ctx, "DELETE FROM background_jobs WHERE user_id = $1", uid) //nolint:errcheck
	})
	t.Cleanup(shutdown.Reset)

	// Pre-insert a running job so the guard blocks a second insert.
	db := &database.DB{Pool: pool}
	firstJobID, inserted, err := db.InsertBackgroundJobGuarded(ctx, uid, "recompute_all_tss", nil, nil, 0)
	if err != nil {
		t.Fatalf("seed running job: %v", err)
	}
	if !inserted {
		t.Fatal("could not seed running job — another job may be running for this user")
	}
	// Ensure the seeded job is completed at test end so the partial unique index
	// does not block unrelated tests for the same user.
	t.Cleanup(func() {
		db.CompleteBackgroundJob(ctx, firstJobID, false, "") //nolint:errcheck
	})

	fake := newFakeInvalidatorWithDone()
	h := &Handler{
		DB:                   db,
		PMC:                  &pmc.Service{DB: db},
		AnalyticsInvalidator: fake,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/recompute-tss", nil)
	req = req.WithContext(withUserAndAdmin(req.Context(), uid, true))
	rec := httptest.NewRecorder()
	h.RecomputeAllTSS(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("already_running: code = %d body = %s, want 200", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["status"] != "already_running" {
		t.Fatalf("response status = %q, want \"already_running\"", resp["status"])
	}

	// No goroutine was launched — the invalidate channel must remain open.
	// Allow a brief window for any unexpected fire.
	select {
	case <-fake.done:
		t.Error("PublishAnalyticsInvalidate was called, but should not be for already_running response")
	case <-time.After(200 * time.Millisecond):
		// Expected: no invalidate fired.
	}

	if calls := fake.callsCopy(); len(calls) != 0 {
		t.Errorf("FakeInvalidator.calls = %v, want []", calls)
	}
}
