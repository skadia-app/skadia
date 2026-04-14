package geocoding

import "fmt"

// SkadiaVersion is the version string embedded in the outbound User-Agent.
// Skadia does not ship a semantic-version constant today (the CI/release
// pipeline is pre-1.0); keeping this here as a single source of truth means
// only one file needs to change when we start tagging releases. Nominatim's
// Usage Policy requires the UA be identifying, not strictly semver-accurate.
const SkadiaVersion = "0.1.0-dev"

// userAgentContact is the static contact URL embedded in the outbound UA.
// OSMF Nominatim-Policy requires an identifying UA with a way to contact the
// operator. For a self-hosting OSS project the project repo URL is the right
// target: abuse reports land in the issue tracker where they can be triaged.
// Admins running sizable instances can point this at their own fork/repo when
// they start tagging releases — the constant is deliberately not
// admin-configurable today because the opt-in Nominatim fallback on tiny
// instances never generates enough traffic to warrant per-instance contact.
const userAgentContact = "https://github.com/skadia"

// CurrentUserAgent returns the User-Agent string for the next outbound
// request. Stable across the process lifetime — no admin-settings lookup.
func CurrentUserAgent() string {
	return fmt.Sprintf("Skadia/%s (+%s)", SkadiaVersion, userAgentContact)
}
