package geocoding

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CacheTTLDays is the time-to-live for cached entries. Negative results
// (provider responded empty) are cached just like positive ones so that
// points at sea / in the desert don't cause repeated outbound requests.
//
// TTL invalidation vs OSM re-import: a fresh OSM import updates the primary
// PostGIS boundary lookup (osm_boundaries), which runs BEFORE any provider
// call. That means a positive PostGIS hit short-circuits before we even
// reach this cache, so stale positive entries are functionally irrelevant
// after re-import. The TTL therefore only matters for the NEGATIVE-cache
// path (see Lookup/Store docs): points where both PostGIS AND the provider
// returned empty. For those, TTL bounds how long a point-at-sea or
// newly-covered rural location stays "no match" before being retried.
const CacheTTLDays = 30

// Cache wraps a pgxpool for the reverse_geocode_cache table. It is safe for
// concurrent use (the DB handles locking).
type Cache struct {
	Pool *pgxpool.Pool
}

// NewCache constructs a Cache bound to the given pool.
func NewCache(pool *pgxpool.Pool) *Cache {
	return &Cache{Pool: pool}
}

// Lookup returns (result, found, err). `found=true, result=non-nil-empty` is a
// valid outcome — it represents a cached "no match" (negative cache) that
// should NOT trigger another outbound request. Entries older than
// CacheTTLDays are treated as misses (they are NOT deleted here; cleanup is
// a future background job).
//
// Negative-cache semantics: when a provider returns no useful address fields
// for a coordinate (ocean points, remote wilderness, coverage gaps), Store
// persists an all-NULL row with the same (lat_key, lng_key, provider) PK.
// Lookup materialises that as a non-nil *Result whose Empty() method
// returns true, and signals found=true so the caller skips the outbound
// request. Without this the hot path would re-fetch the same "no match"
// coordinate on every retry — wasting rate-limiter tokens and risking
// upstream ban for a result that will never change.
//
// The query uses make_interval(days => $4) rather than string-interpolating
// CacheTTLDays so the statement is fully parameterized (prepared-statement
// cacheable, no dynamic SQL).
func (c *Cache) Lookup(ctx context.Context, lat, lng float64, provider string) (*Result, bool, error) {
	if c == nil || c.Pool == nil {
		return nil, false, nil
	}
	latKey := cacheKey(lat)
	lngKey := cacheKey(lng)

	var (
		city        *string
		state       *string
		country     *string
		countryCode *string
	)
	err := c.Pool.QueryRow(ctx, `
		SELECT city, state, country, country_code
		FROM reverse_geocode_cache
		WHERE lat_key = $1 AND lng_key = $2 AND provider = $3
		  AND fetched_at >= NOW() - make_interval(days => $4)`,
		latKey, lngKey, provider, CacheTTLDays,
	).Scan(&city, &state, &country, &countryCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reverse_geocode_cache lookup: %w", err)
	}

	r := &Result{}
	if city != nil {
		r.City = *city
	}
	if state != nil {
		r.State = *state
	}
	if country != nil {
		r.Country = *country
	}
	if countryCode != nil {
		r.CountryCode = *countryCode
	}
	return r, true, nil
}

// Store writes (or upserts) a cache entry. A nil result is persisted as a
// negative-cache entry (all fields NULL). Caller should pass the ORIGINAL
// caller-supplied coordinates; rounding to the cache grid happens here.
func (c *Cache) Store(ctx context.Context, lat, lng float64, provider string, r *Result) error {
	if c == nil || c.Pool == nil {
		return nil
	}
	latKey := cacheKey(lat)
	lngKey := cacheKey(lng)

	var city, state, country, countryCode *string
	if r != nil {
		if r.City != "" {
			s := r.City
			city = &s
		}
		if r.State != "" {
			s := r.State
			state = &s
		}
		if r.Country != "" {
			s := r.Country
			country = &s
		}
		if r.CountryCode != "" {
			s := r.CountryCode
			countryCode = &s
		}
	}

	_, err := c.Pool.Exec(ctx, `
		INSERT INTO reverse_geocode_cache
			(lat_key, lng_key, provider, city, state, country, country_code, fetched_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (lat_key, lng_key, provider) DO UPDATE SET
			city = EXCLUDED.city,
			state = EXCLUDED.state,
			country = EXCLUDED.country,
			country_code = EXCLUDED.country_code,
			fetched_at = EXCLUDED.fetched_at`,
		latKey, lngKey, provider, city, state, country, countryCode,
	)
	if err != nil {
		return fmt.Errorf("reverse_geocode_cache store: %w", err)
	}
	return nil
}
