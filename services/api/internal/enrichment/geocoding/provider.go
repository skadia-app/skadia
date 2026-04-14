// Package geocoding provides external reverse-geocoding providers (Nominatim,
// Photon) as a fallback when the primary PostGIS/osm_boundaries query returns
// no match. All providers share a common interface, a 100 m coordinate-rounding
// privacy layer, a system-wide cache and (where required) a global rate limiter.
package geocoding

import "context"

// ProviderName identifies a geocoding provider. Must match the DB CHECK
// constraint on user_enrichment_settings.geocode_fallback_provider.
const (
	ProviderOff       = "off"
	ProviderNominatim = "nominatim"
	ProviderPhoton    = "photon"
)

// Result is the normalized output of a reverse-geocode query. All fields are
// optional — callers must handle the "not found" case (empty Result).
type Result struct {
	City        string
	State       string
	Country     string
	CountryCode string // uppercase ISO-3166-1 alpha-2
}

// Empty reports whether the result contains no useful data.
func (r *Result) Empty() bool {
	if r == nil {
		return true
	}
	return r.City == "" && r.State == "" && r.Country == "" && r.CountryCode == ""
}

// Provider is the interface implemented by Nominatim and Photon adapters.
// Implementations MUST:
//   - round lat/lng to 3 decimals BEFORE the outbound request (privacy, ~100 m),
//   - honour the per-call context timeout,
//   - set a User-Agent header identifying Skadia,
//   - return (nil, nil) for "no match" (not an error),
//   - return a non-nil error only for network / parse failures.
type Provider interface {
	Name() string
	Reverse(ctx context.Context, lat, lng float64, lang string) (*Result, error)
}

// The User-Agent sent on every outbound request is a static identifier built
// in useragent.go (SkadiaVersion + userAgentContact, both compile-time
// constants). Admin-settings do NOT feed into the UA today — an earlier design
// that pulled app.url / app.contact_email from the runtime SettingsStore was
// rolled back because the opt-in Nominatim fallback on tiny self-host
// instances never generates enough traffic to warrant per-instance contact
// info. If larger installations ever need a distinguishing UA, edit the
// constants in useragent.go rather than reintroducing the settings lookup.
