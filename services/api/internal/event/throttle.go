package event

import (
	"sync"
	"time"
)

// Throttler rate-limits an action per key (e.g. user_id). It is concurrency-
// safe and bounded in memory: at most Cap entries are retained; the oldest
// out-of-window entries are evicted first, with a force-drop fallback when
// every entry is still in-window.
//
// Used by the analytics-invalidate publish path to prevent runaway trigger
// loops (a deterministically-failing worker that re-fires every GET, an
// attacker spamming /health/data, etc.). Centralizing the throttle on the
// publish site means every caller — readiness GET, health-data ingest,
// manual activity, recompute, garmin import — is throttled automatically.
type Throttler struct {
	mu        sync.Mutex
	lastFired map[string]time.Time
	window    time.Duration
	cap       int
}

// NewThrottler constructs a Throttler with the given window and per-key cap.
// A zero or negative window disables throttling (Allow always returns true).
// A zero or negative cap disables eviction (unbounded growth — only sensible
// in tests with a known small key set).
func NewThrottler(window time.Duration, cap int) *Throttler {
	return &Throttler{
		lastFired: make(map[string]time.Time),
		window:    window,
		cap:       cap,
	}
}

// Allow reports whether an action for key may proceed now. It returns true
// at most once per window per key. Uses time.Now as the clock; tests that
// need a deterministic clock should use AllowAt.
func (t *Throttler) Allow(key string) bool {
	return t.AllowAt(key, time.Now())
}

// AllowAt is Allow with an explicit clock, for deterministic tests.
func (t *Throttler) AllowAt(key string, now time.Time) bool {
	if t == nil || t.window <= 0 {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastFired == nil {
		t.lastFired = make(map[string]time.Time)
	}
	if last, ok := t.lastFired[key]; ok && now.Sub(last) < t.window {
		return false
	}
	t.evictIfFullLocked(now)
	t.lastFired[key] = now
	return true
}

// evictIfFullLocked drops entries older than the throttle window when the
// map exceeds cap. Caller must hold t.mu. Worst case: a burst of cap unique
// keys within window — we then drop only the truly-stale ones, keeping the
// throttle effective for the active set.
func (t *Throttler) evictIfFullLocked(now time.Time) {
	if t.cap <= 0 || len(t.lastFired) < t.cap {
		return
	}
	cutoff := now.Add(-t.window)
	for k, ts := range t.lastFired {
		if ts.Before(cutoff) {
			delete(t.lastFired, k)
		}
	}
	// If everything is still in-window (burst of cap+ fresh-now entries),
	// force-drop an arbitrary half to bound memory. Unlike the cutoff-based
	// pass above (which evicts out-of-window entries first), this is purely
	// map-iteration order — no age preference. Throttling becomes best-effort
	// under sustained cap+ unique active keys — acceptable for selfhost scale.
	if len(t.lastFired) >= t.cap {
		drop := len(t.lastFired) / 2
		for k := range t.lastFired {
			if drop == 0 {
				break
			}
			delete(t.lastFired, k)
			drop--
		}
	}
}

// Size returns the current number of tracked keys. Test helper.
func (t *Throttler) Size() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.lastFired)
}
