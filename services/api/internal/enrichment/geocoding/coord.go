package geocoding

import "math"

// RoundCoord rounds a lat or lng to 3 decimal places. The resulting grid
// resolution depends on latitude — 3 decimal places correspond to
// approximately 75–111 m (≈111 m at the equator, ≈55 m at 60°N because a
// degree of longitude shrinks with cos(lat)). Privacy-disclaimer UIs should
// therefore quote the range ("~75–110 m"), not a single "~100 m" figure
// (see frontend/src/pages/SettingsPage.tsx → GEOCODE_STRINGS.privacyNotice).
//
// This rounding is applied BEFORE every outbound HTTP request (privacy,
// de-duplication) and before every cache key computation (so the cache is
// hit regardless of which decimal places the caller passed in).
func RoundCoord(v float64) float64 {
	return math.Round(v*1000) / 1000
}

// cacheKey returns the integer (lat_key, lng_key) used as PK in
// reverse_geocode_cache. Equivalent to int(round(v*1000)).
func cacheKey(v float64) int {
	return int(math.Round(v * 1000))
}
