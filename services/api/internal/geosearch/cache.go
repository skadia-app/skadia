package geosearch

import (
	"sync"
	"time"
)

// Cache is a short-lived in-memory cache for external forward search results.
// It intentionally has no persistence because search queries can include
// private user place names or location context.
type Cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]cacheEntry
	now     func() time.Time
}

type cacheEntry struct {
	expiresAt time.Time
	results   []Result
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		ttl:     ttl,
		entries: map[string]cacheEntry{},
		now:     time.Now,
	}
}

func (c *Cache) get(key string) ([]Result, bool) {
	if c == nil || c.ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return cloneResults(entry.results), true
}

func (c *Cache) set(key string, results []Result) {
	if c == nil || c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		expiresAt: c.now().Add(c.ttl),
		results:   cloneResults(results),
	}
}

func cloneResults(in []Result) []Result {
	if in == nil {
		return nil
	}
	out := make([]Result, len(in))
	copy(out, in)
	return out
}

type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	users  map[string]quotaBucket
}

type quotaBucket struct {
	start time.Time
	used  int
}

func NewLimiter(limit int, window time.Duration, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		limit:  limit,
		window: window,
		now:    now,
		users:  map[string]quotaBucket{},
	}
}

func (l *Limiter) Check(userID string) QuotaState {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return QuotaState{Status: QuotaStatusUnlimited}
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	key := quotaKey(userID)
	bucket := l.bucketForLocked(key)
	remaining := l.limit - bucket.used
	if remaining < 0 {
		remaining = 0
	}
	state := QuotaState{
		Status:       QuotaStatusAvailable,
		LimitPerHour: l.limit,
		Remaining:    remaining,
		ResetAt:      bucket.start.Add(l.window),
	}
	if remaining == 0 {
		state.Status = QuotaStatusRateLimited
		state.BlockingReason = "rate_limited"
	}
	return state
}

func (l *Limiter) Allow(userID string) QuotaState {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return QuotaState{Status: QuotaStatusUnlimited, Used: true}
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	key := quotaKey(userID)
	bucket := l.bucketForLocked(key)
	remaining := l.limit - bucket.used
	if remaining <= 0 {
		return QuotaState{
			Status:         QuotaStatusRateLimited,
			LimitPerHour:   l.limit,
			Remaining:      0,
			ResetAt:        bucket.start.Add(l.window),
			BlockingReason: "rate_limited",
		}
	}

	bucket.used++
	l.users[key] = bucket
	remaining = l.limit - bucket.used
	if remaining < 0 {
		remaining = 0
	}
	return QuotaState{
		Status:       QuotaStatusAvailable,
		LimitPerHour: l.limit,
		Remaining:    remaining,
		ResetAt:      bucket.start.Add(l.window),
		Used:         true,
	}
}

func (l *Limiter) bucketForLocked(key string) quotaBucket {
	now := l.now()
	bucket, ok := l.users[key]
	if !ok || !now.Before(bucket.start.Add(l.window)) {
		bucket = quotaBucket{start: now}
		l.users[key] = bucket
	}
	return bucket
}

func quotaKey(userID string) string {
	if userID == "" {
		return "anonymous"
	}
	return userID
}
