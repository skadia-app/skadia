package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages per-key rate limiters with automatic cleanup.
type RateLimiter struct {
	limiters sync.Map // map[string]*limiterEntry
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a rate limiter and starts a background cleanup goroutine.
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{}
	go rl.cleanup(15 * time.Minute)
	return rl
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string, rps float64, burst int) bool {
	if entry, ok := rl.limiters.Load(key); ok {
		e := entry.(*limiterEntry)
		e.lastSeen = time.Now()
		return e.limiter.Allow()
	}
	// Create new limiter
	l := rate.NewLimiter(rate.Limit(rps), burst)
	rl.limiters.Store(key, &limiterEntry{limiter: l, lastSeen: time.Now()})
	return l.Allow()
}

func (rl *RateLimiter) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		rl.limiters.Range(func(key, value any) bool {
			e := value.(*limiterEntry)
			if time.Since(e.lastSeen) > interval {
				rl.limiters.Delete(key)
			}
			return true
		})
	}
}

// RateLimitLoginByIP rate-limits login attempts: 20 per 15 minutes per IP.
func RateLimitLoginByIP(rl *RateLimiter) func(http.Handler) http.Handler {
	return rateLimitByIP(rl, "login", 20.0/900, 20) // 20 per 900s
}

// RateLimitRegister rate-limits registration: 3 per hour per IP.
func RateLimitRegister(rl *RateLimiter) func(http.Handler) http.Handler {
	return rateLimitByIP(rl, "register", 3.0/3600, 3) // 3 per 3600s
}

// RateLimitForgotPW rate-limits forgot-password: 3 per hour per IP.
func RateLimitForgotPW(rl *RateLimiter) func(http.Handler) http.Handler {
	return rateLimitByIP(rl, "forgotpw", 3.0/3600, 3) // 3 per 3600s
}

// RateLimitGeneric rate-limits generic auth endpoints: 10 per 15 minutes per IP.
func RateLimitGeneric(rl *RateLimiter) func(http.Handler) http.Handler {
	return rateLimitByIP(rl, "generic", 10.0/900, 10) // 10 per 900s
}

func rateLimitByIP(rl *RateLimiter, prefix string, rps float64, burst int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r)
			key := prefix + ":" + ip
			if !rl.Allow(key, rps, burst) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
