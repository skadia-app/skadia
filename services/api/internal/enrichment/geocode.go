package enrichment

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/geocoding"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// osmBoundariesTTL controls how long a "table does not exist" answer is cached.
// After a fresh OSM import the table appears mid-process; without a TTL the
// negative answer sticks for the whole process lifetime and geocoding stays
// broken until the container is restarted. 60 s is long enough to spare the
// information_schema query from per-request pressure but short enough that
// imports propagate in about a minute.
const osmBoundariesTTL = 60 * time.Second

// osmBoundariesCheck caches the "does osm_boundaries exist" check with a TTL.
// On an error (e.g. pool exhausted, context cancelled by the very first caller)
// we do NOT write to lastChecked, so the next caller re-checks immediately. A
// positive result is still cached for TTL seconds. `exists` is tri-state
// through `checked`: `checked=false` means "never successfully probed".
type osmBoundariesCheckState struct {
	mu          sync.Mutex
	exists      atomic.Bool
	checked     atomic.Bool
	lastChecked time.Time
}

var osmBoundariesCheck osmBoundariesCheckState

// resetOsmBoundariesCache clears the cache. Only used in tests.
func resetOsmBoundariesCache() {
	osmBoundariesCheck.mu.Lock()
	defer osmBoundariesCheck.mu.Unlock()
	osmBoundariesCheck.exists.Store(false)
	osmBoundariesCheck.checked.Store(false)
	osmBoundariesCheck.lastChecked = time.Time{}
}

// checkOsmBoundariesExists returns whether the osm_boundaries table is
// available, using a TTL-guarded cache. `probe` is the actual check function,
// injected so tests can supply a stub without needing a real pgxpool.
func checkOsmBoundariesExists(ctx context.Context, probe func(context.Context) (bool, error)) bool {
	// Fast path: last successful check is still within TTL.
	if osmBoundariesCheck.checked.Load() {
		osmBoundariesCheck.mu.Lock()
		fresh := time.Since(osmBoundariesCheck.lastChecked) < osmBoundariesTTL
		osmBoundariesCheck.mu.Unlock()
		if fresh {
			return osmBoundariesCheck.exists.Load()
		}
	}

	// Slow path: serialize re-checks so we don't stampede information_schema.
	osmBoundariesCheck.mu.Lock()
	defer osmBoundariesCheck.mu.Unlock()

	// Another goroutine may have refreshed while we waited for the lock.
	if osmBoundariesCheck.checked.Load() && time.Since(osmBoundariesCheck.lastChecked) < osmBoundariesTTL {
		return osmBoundariesCheck.exists.Load()
	}

	exists, err := probe(ctx)
	if err != nil {
		// Keep any prior cached value; do NOT advance lastChecked so the next
		// caller retries. This prevents a context-cancel in the very first
		// caller from pinning `false` forever (finding #3 / #5).
		slog.Warn("osm_boundaries existence probe failed", "error", err)
		return osmBoundariesCheck.exists.Load() // default false if never checked
	}
	osmBoundariesCheck.exists.Store(exists)
	osmBoundariesCheck.checked.Store(true)
	osmBoundariesCheck.lastChecked = time.Now()
	return exists
}

// GeoResult holds reverse geocoding results.
type GeoResult struct {
	City        string
	State       string
	Country     string
	CountryCode string
}

// ReverseGeocode finds city/state/country for a point.
//
// Primary lookup: PostGIS ST_Contains against osm_boundaries. This requires
// the user to have imported OSM data for the region — if the table is absent
// or the query is empty, we fall back (optionally) to an external provider
// configured per user via user_enrichment_settings.geocode_fallback_provider.
//
// The fallback is strictly opt-in; default is "off". All external requests go
// through a system-wide cache (reverse_geocode_cache) with 100 m coordinate
// rounding, and Nominatim additionally through a global 1-req/s rate limiter.
//
// userID may be empty — the external fallback is simply skipped in that case.
//
// Callers that reverse-geocode in a tight loop (e.g. the geocoding backfill
// which iterates up to thousands of activities) should pre-load the user's
// enrichment settings once and call ReverseGeocodeWithSettings instead — this
// entry point re-reads settings on every invocation, which is fine for single
// enrichments but wastes O(N) DB queries in a batch job (finding #72).
func ReverseGeocode(ctx context.Context, db *database.DB, userID string, lat, lng float64) (*GeoResult, error) {
	return ReverseGeocodeWithSettings(ctx, db, userID, nil, lat, lng)
}

// ReverseGeocodeWithSettings is identical to ReverseGeocode but accepts a
// pre-loaded *model.EnrichmentSettings to avoid re-reading from the DB on
// every call. Pass nil for single-shot usage (the function then loads settings
// itself via the external-fallback code path, matching ReverseGeocode).
//
// The pre-loaded settings snapshot is only consulted when PostGIS has no city
// match and the external fallback fires. Stale settings (e.g. provider turned
// off by the user mid-run) are the caller's responsibility: the geocoding
// backfill handler re-reads + re-passes fresh settings every 25 iterations.
func ReverseGeocodeWithSettings(ctx context.Context, db *database.DB, userID string, settings *model.EnrichmentSettings, lat, lng float64) (*GeoResult, error) {
	result := postgisReverseGeocode(ctx, db.Pool, lat, lng)

	// Fallback only runs if PostGIS didn't produce a city. State/country
	// without a city is not useful for start_city enrichment, so we still
	// invoke the fallback in that case.
	if result != nil && result.City != "" {
		return result, nil
	}

	if userID == "" {
		return result, nil
	}

	fallback := externalReverseGeocode(ctx, db, userID, settings, lat, lng)
	if fallback == nil {
		return result, nil
	}

	// Merge: PostGIS wins for any non-empty field, external fills gaps.
	if result == nil {
		result = &GeoResult{}
	}
	if result.City == "" {
		result.City = fallback.City
	}
	if result.State == "" {
		result.State = fallback.State
	}
	if result.Country == "" {
		result.Country = fallback.Country
	}
	if result.CountryCode == "" {
		result.CountryCode = fallback.CountryCode
	}

	if result.City == "" && result.State == "" && result.Country == "" && result.CountryCode == "" {
		return nil, nil
	}
	return result, nil
}

// postgisReverseGeocode runs the osm_boundaries ST_Contains query. Returns nil
// when the table is absent or the query returned nothing useful. Never errors
// upward — on query failure, returns nil and logs a warning (matches pre-
// existing behaviour so import pipelines never fail on geocoding).
func postgisReverseGeocode(ctx context.Context, pool *pgxpool.Pool, lat, lng float64) *GeoResult {
	// Check if osm_boundaries table exists — cached with a TTL (see
	// osmBoundariesTTL). `sync.Once` was previously used here but had two bugs:
	//   #3: a transient error on the very first probe (cancelled context, pool
	//       not ready at startup) pinned `false` for the process lifetime.
	//   #2: after a fresh OSM import that creates the table, the cached `false`
	//       never re-checks, so geocoding stays broken until the API restarts.
	// The TTL-based check below solves both: errors leave lastChecked untouched
	// so the next caller retries, and positive results expire after 60 s.
	exists := checkOsmBoundariesExists(ctx, func(c context.Context) (bool, error) {
		var ok bool
		err := pool.QueryRow(c, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_name = 'osm_boundaries'
			)`,
		).Scan(&ok)
		return ok, err
	})
	if !exists {
		return nil
	}

	result := &GeoResult{}

	// Query for city (admin_level 8), state (admin_level 4), country (admin_level 2).
	// Note: osm_boundaries is recreated by osm2pgsql --create from the Lua schema
	// on every OSM import, so the table only has columns defined in skadia-osm.lua.
	// country_code is NOT part of that schema — derive it from country name if needed.
	//
	// TODO (i18n): `name` is the OSM primary-language label ("Ελλάς", "Roma", "Αθήνα").
	// Localization is NOT yet implemented on either path of ReverseGeocode:
	//   - PostGIS (this function): always returns the primary-language OSM name.
	//   - External fallback (externalReverseGeocode): forwards Accept-Language
	//     "de,en" but that only fires when PostGIS found nothing (the "Option B
	//     gap fill" case). No language-aware lookup exists when PostGIS matched.
	//
	// When the UI gains a per-user language setting, two paths are possible.
	// Neither exists yet:
	//
	//   (A) Server-side — extend skadia-osm.lua to also store `tags` JSONB (or
	//       explicit name:de / name:en columns) on osm_boundaries; change this
	//       query to COALESCE(tags->>'name:'||$lang, name). Requires full OSM
	//       re-import. Best for large self-host instances (fast, private, offline).
	//
	//   (B) Client-side — route a request with Accept-Language through the
	//       external Nominatim/Photon fallback even when PostGIS already has a
	//       match. Cheap to implement, works today, but adds external requests
	//       for data we technically already hold. Best for small instances.
	//
	// Both are non-trivial changes. Pick one when i18n work starts.
	// admin_level 5 is included here to stay consistent with enrichGeoTagsBorders
	// (which already accepts 5 for Greek Περιφέρεια borders, finding #61). Without
	// it, a Cretan activity would get a "Kreta" border crossing but no state tag.
	// Per-country handling happens in the switch below (finding #73).
	rows, err := pool.Query(ctx, `
		SELECT admin_level, name
		FROM osm_boundaries
		WHERE ST_Contains(geom, ST_SetSRID(ST_MakePoint($1, $2), 4326))
		  AND admin_level IN (2, 4, 5, 6, 7, 8)
		ORDER BY admin_level DESC`,
		lng, lat,
	)
	if err != nil {
		slog.Warn("reverse geocode query failed", "error", err)
		return nil
	}
	defer rows.Close()

	// Admin-level conventions vary per country:
	//   DE/AT/IT/CH/FR/ES: municipalities on level 8
	//   GR:                municipalities (Δήμος) on level 7, level 8 is for sub-
	//                      units — without level 7 we never get a city name
	//   DE:                level 7 is Verbandsgemeinde (rarely used for display)
	//
	// We walk results ORDER BY admin_level DESC and take the first non-empty
	// label at levels 8 > 7 > 6 for "city". This gives us the deepest matching
	// municipality regardless of the country-specific level convention.
	//
	// State handling — admin_level 4 vs 5 (finding #73):
	//   The two levels carry different semantics per country. Based on the
	//   current osm_boundaries import (DE, CH, GR, Peru):
	//     DE:  4 = Bundesland (correct State)
	//          5 = Regierungsbezirk (sub-division, WRONG as State)
	//     CH:  4 = Kanton (correct State)
	//          5 = Amtei / Verwaltungsregion (sub-canton, WRONG)
	//     GR:  4 = "Αποκεντρωμένη Διοίκηση" (decentralized admin, bureaucratic —
	//              NOT the political region the user expects)
	//          5 = "Περιφέρεια" (the actual Greek region, correct)
	//     Peru: 4 = Región (correct), 5 = one outlier ("Lima Metropolitana")
	//   Strategy: prefer admin_4 in general, but override it with admin_5 when
	//   admin_4 starts with the Greek-bureaucracy prefix "Αποκεντρωμένη". Because
	//   rows arrive ORDER BY admin_level DESC, level 5 reaches us BEFORE level 4,
	//   so we stash level 5 first and then decide at level 4.
	const grBureaucracyPrefix = "Αποκεντρωμένη"
	var adminLevel5Name string
	for rows.Next() {
		var level int
		var name string
		if err := rows.Scan(&level, &name); err != nil {
			continue
		}
		switch level {
		case 8, 7, 6:
			if result.City == "" {
				result.City = name
			}
		case 5:
			// Stash — decided at level 4 (or used as fallback if no level 4).
			adminLevel5Name = name
		case 4:
			if strings.HasPrefix(name, grBureaucracyPrefix) && adminLevel5Name != "" {
				// GR case: admin_4 is the bureaucratic "Αποκεντρωμένη Διοίκηση";
				// the real Περιφέρεια sits at admin_5.
				result.State = adminLevel5Name
			} else {
				result.State = name
			}
		case 2:
			result.Country = name
		}
	}
	// No admin_4 row at all (e.g. country with only admin_5 subdivisions) —
	// fall back to the stashed level-5 name.
	if result.State == "" && adminLevel5Name != "" {
		result.State = adminLevel5Name
	}

	if result.City == "" && result.State == "" && result.Country == "" {
		return nil
	}
	return result
}

// externalReverseGeocode performs the opt-in external provider lookup. Returns
// nil when the user has the feature disabled ("off"), when the cache/provider
// call fails, or when neither yields data. On success it also writes the
// result back to the cache (including negative results).
//
// `preloaded` may be nil — in that case the function loads enrichment settings
// itself (matches the single-shot path used by Stage-1). Callers that batch-
// process many activities should pass a pre-loaded snapshot to avoid O(N) DB
// queries against user_enrichment_settings (finding #72).
func externalReverseGeocode(ctx context.Context, db *database.DB, userID string, preloaded *model.EnrichmentSettings, lat, lng float64) *GeoResult {
	settings := preloaded
	if settings == nil {
		loaded, err := db.GetEnrichmentSettings(ctx, userID)
		if err != nil {
			slog.Warn("reverse geocode fallback: load settings failed", "user_id", userID, "error", err)
			return nil
		}
		settings = loaded
	}
	if settings == nil || settings.GeocodeFallbackProvider == "" || settings.GeocodeFallbackProvider == geocoding.ProviderOff {
		return nil
	}

	provider, err := buildProvider(settings)
	if err != nil {
		slog.Warn("reverse geocode fallback: build provider failed", "error", err, "provider", settings.GeocodeFallbackProvider)
		return nil
	}
	providerName := provider.Name()

	cache := geocoding.NewCache(db.Pool)

	// Cache hit → return early (including cached negative results).
	if cached, found, err := cache.Lookup(ctx, lat, lng, providerName); err != nil {
		slog.Warn("reverse geocode cache lookup failed", "error", err)
	} else if found {
		if cached.Empty() {
			return nil
		}
		return convertResult(cached)
	}

	// Outbound request (rate-limited inside the provider for Nominatim).
	result, err := provider.Reverse(ctx, lat, lng, "de,en")
	if err != nil {
		slog.Warn("reverse geocode fallback request failed",
			"provider", providerName, "error", err, "lat", lat, "lng", lng)
		return nil
	}

	// Cache BOTH positive and negative results.
	if storeErr := cache.Store(ctx, lat, lng, providerName, result); storeErr != nil {
		slog.Warn("reverse geocode cache store failed", "error", storeErr)
	}
	if result == nil || result.Empty() {
		return nil
	}
	return convertResult(result)
}

func buildProvider(s *model.EnrichmentSettings) (geocoding.Provider, error) {
	timeout := time.Duration(s.GeocodeFallbackTimeoutS) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	var customURL string
	if s.GeocodeFallbackURL != nil {
		customURL = *s.GeocodeFallbackURL
	}

	switch s.GeocodeFallbackProvider {
	case geocoding.ProviderNominatim:
		return &geocoding.Nominatim{BaseURL: customURL, Timeout: timeout}, nil
	case geocoding.ProviderPhoton:
		return &geocoding.Photon{BaseURL: customURL, Timeout: timeout}, nil
	default:
		return nil, errProviderUnknown
	}
}

var errProviderUnknown = &providerUnknownError{}

type providerUnknownError struct{}

func (e *providerUnknownError) Error() string { return "unknown geocode fallback provider" }

func convertResult(r *geocoding.Result) *GeoResult {
	if r == nil {
		return nil
	}
	return &GeoResult{
		City:        r.City,
		State:       r.State,
		Country:     r.Country,
		CountryCode: r.CountryCode,
	}
}

