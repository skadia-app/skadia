package geocoding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestPhotonReverse_FirstFeature(t *testing.T) {
	body := `{
		"features":[
			{"properties":{"city":"Salzburg","state":"Salzburg","country":"Austria","countrycode":"at"}},
			{"properties":{"city":"Should-Be-Ignored"}}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "Skadia") {
			t.Errorf("missing Skadia User-Agent, got %q", got)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	// httptest binds to 127.0.0.1 — opt in to bypass the SSRF validator.
	bypassValidationForTest(t)

	p := &Photon{BaseURL: srv.URL, Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	got, err := p.Reverse(context.Background(), 47.8, 13.04, "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("result is nil")
	}
	if got.City != "Salzburg" {
		t.Errorf("city = %q, want Salzburg", got.City)
	}
	if got.CountryCode != "AT" {
		t.Errorf("country_code = %q, want AT", got.CountryCode)
	}
}

// Photon often omits city for rural points; county must take over.
func TestPhotonReverse_CityFallsBackToCounty(t *testing.T) {
	body := `{"features":[{"properties":{"county":"Zillertal","country":"Austria","countrycode":"at"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	p := &Photon{BaseURL: srv.URL, Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	got, err := p.Reverse(context.Background(), 47.2, 11.9, "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.City != "Zillertal" {
		t.Fatalf("city fallback failed, got %+v", got)
	}
}

// TestPhotonReverse_RejectsPrivateBaseURL is the integration-level SSRF guard
// for Photon (finding #10).
func TestPhotonReverse_RejectsPrivateBaseURL(t *testing.T) {
	p := &Photon{BaseURL: "http://169.254.169.254/latest/meta-data/", Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	_, err := p.Reverse(context.Background(), 1, 1, "de")
	if err == nil {
		t.Fatal("expected SSRF-rejection error for AWS metadata URL, got nil")
	}
	if !strings.Contains(err.Error(), "invalid geocoding base URL") {
		t.Errorf("error %q should mention invalid base URL", err.Error())
	}
}

func TestPhotonReverse_NoFeatures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"features":[]}`))
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	p := &Photon{BaseURL: srv.URL, Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	got, err := p.Reverse(context.Background(), 0, 0, "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result for empty features, got %+v", got)
	}
}

// TestPhotonReverse_RateLimiterWaits verifies finding #16: Photon outbound
// calls are gated through photonLimiter. We install a deliberately tight
// bucket (1 req/s, burst=1), fire two requests back-to-back, and assert
// the second call blocks on the limiter for a measurable delay. This is a
// behavioural test — it confirms the wire-up, not the absolute timing
// (slack bounds chosen to stay CI-safe).
func TestPhotonReverse_RateLimiterWaits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"features":[{"properties":{"city":"X","countrycode":"at"}}]}`))
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	// Swap in a strict 1 req/s bucket for the duration of this test and
	// restore the production defaults on cleanup.
	origLimit := photonLimiter.Limit()
	origBurst := photonLimiter.Burst()
	photonLimiter.SetLimit(rate.Limit(1))
	photonLimiter.SetBurst(1)
	t.Cleanup(func() {
		photonLimiter.SetLimit(origLimit)
		photonLimiter.SetBurst(origBurst)
	})

	p := &Photon{BaseURL: srv.URL, Timeout: 2 * time.Second}

	// Drain the bucket: first call consumes the sole token.
	if _, err := p.Reverse(context.Background(), 1, 1, "de"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Second call must wait ~1s for the next token. Measure the wait.
	start := time.Now()
	if _, err := p.Reverse(context.Background(), 1, 1, "de"); err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	waited := time.Since(start)
	// Lower bound: comfortably under 1s would mean the limiter was bypassed.
	// Upper bound: pick a generous ceiling to keep the test non-flaky on
	// slow CI runners. The point is to prove wait > ~half a second.
	if waited < 400*time.Millisecond {
		t.Errorf("second Reverse returned after %v, expected ~1s wait through photonLimiter", waited)
	}
	if waited > 5*time.Second {
		t.Errorf("second Reverse took %v, far longer than the 1s bucket wait", waited)
	}
}

// TestPhotonReverse_BaseURLWithFragment verifies finding #9 for Photon too:
// a fragment on the base URL must not swallow the outbound query string.
func TestPhotonReverse_BaseURLWithFragment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the fragment had swallowed the query, r.URL.Query() would be empty.
		if r.URL.Query().Get("lat") == "" {
			t.Errorf("server received no lat param — fragment likely swallowed query: %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("radius") != "1" {
			t.Errorf("radius param = %q, want 1", r.URL.Query().Get("radius"))
		}
		_, _ = w.Write([]byte(`{"features":[{"properties":{"city":"X","countrycode":"at"}}]}`))
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	p := &Photon{BaseURL: srv.URL + "/reverse#frag", Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	got, err := p.Reverse(context.Background(), 47.2, 11.4, "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.City != "X" {
		t.Fatalf("result = %+v, want City=X", got)
	}
}
