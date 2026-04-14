package geocoding

import "testing"

// withRelaxedLimiters widens both process-wide rate limiters to a generous
// burst for the duration of a test, and restores the production defaults
// (Nominatim burst=1, Photon burst=5) via t.Cleanup. Every test that does a
// real provider round-trip MUST use this helper — otherwise:
//
//   - adjacent tests serialize through the global 1-req/s Nominatim bucket
//     (slow, flaky under -race),
//   - a widened burst leaks across tests, masking real production behaviour
//     and hiding regressions in other tests that depend on the strict
//     default (e.g. a future TestNominatimBackoff).
//
// Keep the cleanup values in sync with the constants in ratelimit.go.
func withRelaxedLimiters(t *testing.T) {
	t.Helper()
	nominatimLimiter.SetBurst(10)
	photonLimiter.SetBurst(10)
	t.Cleanup(func() {
		nominatimLimiter.SetBurst(1)
		photonLimiter.SetBurst(5)
	})
}
