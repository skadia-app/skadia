package event

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestThrottler_AllowWithinWindow — first call admits, second call within
// the window for the same key is suppressed.
func TestThrottler_AllowWithinWindow(t *testing.T) {
	thr := NewThrottler(60*time.Second, 10_000)
	uid := "user-1"
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if !thr.AllowAt(uid, t0) {
		t.Fatal("first call must admit")
	}
	if thr.AllowAt(uid, t0.Add(30*time.Second)) {
		t.Error("call within window must be suppressed")
	}
	if thr.AllowAt(uid, t0.Add(59*time.Second)) {
		t.Error("call still within window must be suppressed")
	}
}

// TestThrottler_WindowExpiry — after the window has elapsed, a fresh call
// for the same key admits again.
func TestThrottler_WindowExpiry(t *testing.T) {
	thr := NewThrottler(60*time.Second, 10_000)
	uid := "user-1"
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if !thr.AllowAt(uid, t0) {
		t.Fatal("first call must admit")
	}
	if thr.AllowAt(uid, t0.Add(30*time.Second)) {
		t.Error("call within window must be suppressed")
	}
	if !thr.AllowAt(uid, t0.Add(61*time.Second)) {
		t.Error("call after window must admit again")
	}
}

// TestThrottler_DifferentKeysIndependent — throttling is per-key.
func TestThrottler_DifferentKeysIndependent(t *testing.T) {
	thr := NewThrottler(60*time.Second, 10_000)
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	if !thr.AllowAt("user-A", t0) {
		t.Fatal("user-A first call must admit")
	}
	if !thr.AllowAt("user-B", t0) {
		t.Error("user-B must admit independently of user-A")
	}
}

// TestThrottler_MemoryCap — when the map exceeds cap, eviction keeps it
// bounded. Uses a small synthetic load and verifies the map shrinks.
func TestThrottler_MemoryCap(t *testing.T) {
	const cap = 100
	thr := NewThrottler(60*time.Second, cap)
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// Fill to cap with old entries.
	for i := 0; i < cap; i++ {
		thr.AllowAt(fmt.Sprintf("u-%d", i), t0)
	}
	if got := thr.Size(); got != cap {
		t.Fatalf("pre-eviction size = %d, want %d", got, cap)
	}

	// Far-future call: cutoff pass drops every prior entry, then admits new one.
	thr.AllowAt("u-new", t0.Add(5*time.Minute))
	if got := thr.Size(); got > cap {
		t.Errorf("post-eviction size = %d, must stay ≤ %d", got, cap)
	}
}

// TestThrottler_ForceDropBranch — when the map is at cap and every entry is
// still in-window (no out-of-window entries to evict), the force-drop branch
// reduces the map to at most cap entries.
func TestThrottler_ForceDropBranch(t *testing.T) {
	const cap = 100
	thr := NewThrottler(60*time.Second, cap)
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	// Fill to cap with fresh-now entries (all in-window).
	for i := 0; i < cap; i++ {
		thr.AllowAt(fmt.Sprintf("u-%d", i), t0)
	}
	if got := thr.Size(); got != cap {
		t.Fatalf("pre-burst size = %d, want %d", got, cap)
	}

	// Burst more unique keys at the same t0: cutoff pass is a no-op (all
	// in-window), force-drop reduces the map before admitting each new caller.
	for i := 0; i < cap; i++ {
		thr.AllowAt(fmt.Sprintf("burst-%d", i), t0)
	}
	if got := thr.Size(); got > cap {
		t.Errorf("post-force-drop size = %d, must be ≤ %d (force-drop branch failed)", got, cap)
	}
}

// TestThrottler_NilSafe — a nil *Throttler always admits. Lets callers like
// Publisher embed an optional throttler without nil-checking.
func TestThrottler_NilSafe(t *testing.T) {
	var thr *Throttler
	if !thr.Allow("anything") {
		t.Error("nil Throttler must admit")
	}
}

// TestThrottler_ZeroWindowDisables — window <= 0 disables throttling.
func TestThrottler_ZeroWindowDisables(t *testing.T) {
	thr := NewThrottler(0, 10_000)
	for i := 0; i < 5; i++ {
		if !thr.Allow("user-1") {
			t.Errorf("call #%d must admit when throttling is disabled", i)
		}
	}
}

// TestThrottler_RaceFreeUnderConcurrentLoad — many goroutines hammering the
// same key must produce exactly one admit. Run with -race.
func TestThrottler_RaceFreeUnderConcurrentLoad(t *testing.T) {
	thr := NewThrottler(60*time.Second, 10_000)
	const goroutines = 64
	const perGoroutine = 100

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		admitted int
	)
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				if thr.AllowAt("hot-key", t0) {
					mu.Lock()
					admitted++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if admitted != 1 {
		t.Errorf("admitted = %d, want exactly 1 across %d goroutines",
			admitted, goroutines*perGoroutine)
	}
}

// throttledFakeInvalidator wraps a FakeInvalidator with a Throttler. Models
// the production wiring (Publisher.AnalyticsThrottle) without requiring a
// live NATS connection, so cross-layer tests can verify that two call sites
// hitting the same invalidator only produce one wire-level publish.
type throttledFakeInvalidator struct {
	inner *FakeInvalidator
	t     *Throttler
}

func (f *throttledFakeInvalidator) PublishAnalyticsInvalidate(userID string) error {
	if f.t != nil && !f.t.Allow(userID) {
		return nil
	}
	return f.inner.PublishAnalyticsInvalidate(userID)
}

// TestThrottler_CrossLayerSuppression — the value of this refactor: when
// two unrelated trigger sites (e.g. blood-pressure ingest and pulse ingest in
// health_data.go) call PublishAnalyticsInvalidate within the throttle window,
// only ONE call reaches the wire. Before R2-L5 each handler called the
// publisher directly; only the readiness GET path was throttled, so a
// 100/s-spam attack on /health/data could flood NATS.
func TestThrottler_CrossLayerSuppression(t *testing.T) {
	fake := &FakeInvalidator{}
	thr := NewThrottler(60*time.Second, 10_000)
	inv := &throttledFakeInvalidator{inner: fake, t: thr}

	uid := "user-A"

	// Site 1: blood_pressure ingest fires the trigger.
	if err := inv.PublishAnalyticsInvalidate(uid); err != nil {
		t.Fatalf("site 1 publish: %v", err)
	}
	// Site 2: pulse / HRV / sleep ingest fires immediately after.
	if err := inv.PublishAnalyticsInvalidate(uid); err != nil {
		t.Fatalf("site 2 publish: %v", err)
	}
	// Site 3: same user, third trigger from yet another path.
	if err := inv.PublishAnalyticsInvalidate(uid); err != nil {
		t.Fatalf("site 3 publish: %v", err)
	}

	if got := len(fake.CallsCopy()); got != 1 {
		t.Errorf("cross-layer wire calls = %d, want 1 (throttler must suppress 2nd+3rd publish)", got)
	}

	// A different user is independent.
	if err := inv.PublishAnalyticsInvalidate("user-B"); err != nil {
		t.Fatalf("user-B publish: %v", err)
	}
	if got := len(fake.CallsCopy()); got != 2 {
		t.Errorf("after user-B publish, wire calls = %d, want 2", got)
	}
}
