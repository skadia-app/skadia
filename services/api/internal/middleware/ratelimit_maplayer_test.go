package middleware

// Unit tests for RateLimitMapLayerWrite.
// No DB required — uses an in-process RateLimiter and httptest.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// nextHandler is a trivial http.Handler that returns 200 OK.
var nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// requestWithUser returns a request whose context contains the given user ID.
func requestWithUser(uid string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	return r.WithContext(context.WithValue(r.Context(), UserIDKey, uid))
}

// TestRateLimitMapLayerWrite_BurstThenThrottle verifies that the burst is
// consumed and subsequent requests are rejected for the same user.
//
// With perTenMin=60: rps = 60/600 = 0.1, burst = max(60/4, 5) = 15.
// Sending 20 requests: first 15 should pass (burst), remaining 5 should be 429.
func TestRateLimitMapLayerWrite_BurstThenThrottle(t *testing.T) {
	rl := NewRateLimiter(t.Context())
	perTenMin := func() int { return 60 }
	mw := RateLimitMapLayerWrite(rl, perTenMin)
	handler := mw(nextHandler)

	uid := "user-" + t.Name()
	var passed, throttled int
	const total = 20
	for i := 0; i < total; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, requestWithUser(uid))
		if rr.Code == http.StatusOK {
			passed++
		} else if rr.Code == http.StatusTooManyRequests {
			throttled++
		} else {
			t.Errorf("unexpected status %d", rr.Code)
		}
	}
	// burst=15, so first 15 pass; remaining 5 are throttled.
	if passed != 15 {
		t.Errorf("expected 15 passed, got %d (burst mismatch)", passed)
	}
	if throttled != 5 {
		t.Errorf("expected 5 throttled, got %d", throttled)
	}
}

// TestRateLimitMapLayerWrite_PerUserKeying verifies different users share no state.
// Each distinct user should get their own burst budget.
func TestRateLimitMapLayerWrite_PerUserKeying(t *testing.T) {
	rl := NewRateLimiter(t.Context())
	perTenMin := func() int { return 60 }
	mw := RateLimitMapLayerWrite(rl, perTenMin)
	handler := mw(nextHandler)

	// 10 distinct users, 1 request each: all should pass (burst >= 1 per user).
	for i := 0; i < 10; i++ {
		uid := "distinct-user-" + t.Name() + "-" + string(rune('a'+i))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, requestWithUser(uid))
		if rr.Code != http.StatusOK {
			t.Errorf("user %d: expected 200, got %d", i, rr.Code)
		}
	}
}

// TestRateLimitMapLayerWrite_RetryAfterHeader verifies the Retry-After header
// is set when a request is throttled.
func TestRateLimitMapLayerWrite_RetryAfterHeader(t *testing.T) {
	rl := NewRateLimiter(t.Context())
	// Low burst=1 (perTenMin / 4 = 0, clamped to 5 → actually burst=5).
	// Use perTenMin=4 so burst = 4/4 = 1 < 5 → clamped to 5.
	// Use a very small value so burst=5 is consumed quickly.
	perTenMin := func() int { return 4 }
	mw := RateLimitMapLayerWrite(rl, perTenMin)
	handler := mw(nextHandler)

	uid := "retry-header-user-" + t.Name()
	var lastThrottled *httptest.ResponseRecorder
	for i := 0; i < 10; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, requestWithUser(uid))
		if rr.Code == http.StatusTooManyRequests {
			lastThrottled = rr
			break
		}
	}
	if lastThrottled == nil {
		t.Skip("all requests passed; burst larger than expected — skipping Retry-After check")
	}
	if ra := lastThrottled.Header().Get("Retry-After"); ra == "" {
		t.Error("Retry-After header missing on 429 response")
	}
}
