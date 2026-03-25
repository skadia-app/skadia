package middleware

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// 500K: Ausgelegt für 100K User mit multiplen IPs + User-IDs als Keys.
const maxRateLimitEntries = 500_000

// RateLimiter manages per-key rate limiters with automatic cleanup.
type RateLimiter struct {
	limiters sync.Map // map[string]*limiterEntry
	size     atomic.Int64
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a rate limiter and starts a background cleanup goroutine.
// The cleanup goroutine stops when ctx is cancelled.
func NewRateLimiter(ctx context.Context) *RateLimiter {
	rl := &RateLimiter{}
	go rl.cleanup(ctx, 15*time.Minute)
	return rl
}

// Allow checks if a request from the given key is allowed.
func (rl *RateLimiter) Allow(key string, rps float64, burst int) bool {
	// Fast path: key already exists.
	if entry, ok := rl.limiters.Load(key); ok {
		e := entry.(*limiterEntry)
		e.lastSeen = time.Now()
		return e.limiter.Allow()
	}

	// Cap map size to prevent memory exhaustion under attack.
	if rl.size.Load() >= maxRateLimitEntries {
		return true // fail open — DoS protection shouldn't become DoS
	}

	// Use LoadOrStore to avoid race between concurrent first-time callers.
	newEntry := &limiterEntry{
		limiter:  rate.NewLimiter(rate.Limit(rps), burst),
		lastSeen: time.Now(),
	}
	actual, loaded := rl.limiters.LoadOrStore(key, newEntry)
	e := actual.(*limiterEntry)
	e.lastSeen = time.Now()
	if !loaded {
		rl.size.Add(1)
	}
	return e.limiter.Allow()
}

func (rl *RateLimiter) cleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.limiters.Range(func(key, value any) bool {
				e := value.(*limiterEntry)
				if time.Since(e.lastSeen) > interval {
					rl.limiters.Delete(key)
					rl.size.Add(-1)
				}
				return true
			})
		case <-ctx.Done():
			return
		}
	}
}

// normalizeIPForRateLimit normalizes IPs for rate limiting keys.
// IPv6 addresses are reduced to /64 prefix to prevent per-address rotation attacks.
func normalizeIPForRateLimit(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	// IPv4: use full address.
	if parsed.To4() != nil {
		return parsed.String()
	}
	// IPv6: use /64 prefix.
	mask := net.CIDRMask(64, 128)
	return parsed.Mask(mask).String()
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
			ip := normalizeIPForRateLimit(ClientIP(r))
			key := prefix + ":" + ip
			if !rl.Allow(key, rps, burst) {
				writeJSONError(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
