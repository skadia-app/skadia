package geocoding

import "testing"

// bypassValidationForTest disables the SSRF validator for the duration of a
// test. Provider tests need this because httptest.NewServer binds to
// 127.0.0.1, which the validator rejects in production. Automatically
// restored via t.Cleanup so tests stay isolated.
//
// Prefer this over building per-test public-looking DNS entries or custom
// Listeners: the validator is exercised directly in validate_test.go, so the
// provider tests can trust that layer without re-testing it.
func bypassValidationForTest(t *testing.T) {
	t.Helper()
	validationBypassForTest.Store(true)
	t.Cleanup(func() {
		validationBypassForTest.Store(false)
	})
}
