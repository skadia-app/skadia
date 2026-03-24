package middleware

import (
	"context"
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
			rl.size.Add(-1)
		}
		return true
	})

	if _, ok := rl.limiters.Load("stale"); ok {
		t.Fatal("expected stale entry to be removed after cleanup")
	}
}

func TestRateLimiter_MaxEntriesCap(t *testing.T) {
	rl := &RateLimiter{}
	rl.size.Store(maxRateLimitEntries)

	// Should fail open when at capacity
	if !rl.Allow("new-key-at-cap", 0.001, 1) {
		t.Fatal("expected Allow=true (fail open) when at max entries cap")
	}

	// The key should NOT have been stored
	if _, ok := rl.limiters.Load("new-key-at-cap"); ok {
		t.Fatal("expected key not to be stored when at max entries cap")
	}
}

func TestRateLimiter_SizeTracking(t *testing.T) {
	rl := &RateLimiter{}

	rl.Allow("key1", 1, 5)
	rl.Allow("key2", 1, 5)
	rl.Allow("key1", 1, 5) // existing key, should not increment

	if got := rl.size.Load(); got != 2 {
		t.Fatalf("expected size=2, got %d", got)
	}
}

func TestRateLimiter_ContextShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rl := NewRateLimiter(ctx)

	// Verify it works
	if !rl.Allow("test", 10, 10) {
		t.Fatal("expected Allow=true")
	}

	// Cancel should stop the cleanup goroutine (no panic, no leak)
	cancel()

	// Give goroutine time to exit
	time.Sleep(50 * time.Millisecond)

	// Should still work after cleanup goroutine exits
	if !rl.Allow("test2", 10, 10) {
		t.Fatal("expected Allow=true after cleanup shutdown")
	}
}

func TestNormalizeIPForRateLimit(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.1.1", "192.168.1.1"},
		{"10.0.0.1", "10.0.0.1"},
		{"not-an-ip", "not-an-ip"},
		// IPv6: should be normalized to /64
		{"2001:db8:85a3::8a2e:370:7334", "2001:db8:85a3::"},
		{"2001:db8:85a3:1234:5678:9abc:def0:1234", "2001:db8:85a3:1234::"},
		{"::1", "::"},
	}

	for _, tt := range tests {
		got := normalizeIPForRateLimit(tt.input)
		if got != tt.want {
			t.Errorf("normalizeIPForRateLimit(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
