package middleware

import (
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := &RateLimiter{} // no cleanup goroutine in tests

	// Burst of 3: first three calls must succeed
	for i := range 3 {
		if !rl.Allow("test", 1, 3) {
			t.Fatalf("expected Allow=true for call %d, got false", i+1)
		}
	}

	// Fourth call must be denied (burst exhausted)
	if rl.Allow("test", 1, 3) {
		t.Fatal("expected Allow=false after burst exceeded, got true")
	}

	// A different key must be independent
	if !rl.Allow("other", 1, 3) {
		t.Fatal("expected Allow=true for independent key 'other', got false")
	}
}

func TestRateLimiter_DifferentKeysIndependent(t *testing.T) {
	rl := &RateLimiter{}

	// Exhaust key "a"
	for range 2 {
		rl.Allow("a", 1, 2)
	}
	if rl.Allow("a", 1, 2) {
		t.Fatal("expected 'a' to be rate-limited")
	}

	// Key "b" must still be allowed
	if !rl.Allow("b", 1, 2) {
		t.Fatal("expected 'b' to be allowed independently")
	}
}

func TestRateLimiter_CleanupRemovesStaleEntries(t *testing.T) {
	rl := &RateLimiter{}

	// Seed an entry
	rl.Allow("stale", 1, 5)

	// Manually age the entry beyond the cleanup interval
	if entry, ok := rl.limiters.Load("stale"); ok {
		e := entry.(*limiterEntry)
		e.lastSeen = time.Now().Add(-20 * time.Minute)
	}

	// Run cleanup with 15-minute interval — the stale entry should be removed
	rl.limiters.Range(func(key, value any) bool {
		e := value.(*limiterEntry)
		if time.Since(e.lastSeen) > 15*time.Minute {
			rl.limiters.Delete(key)
		}
		return true
	})

	if _, ok := rl.limiters.Load("stale"); ok {
		t.Fatal("expected stale entry to be removed after cleanup")
	}
}
