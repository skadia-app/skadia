package geocoding

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestNominatimReverse_CityFallback verifies the city/town/village/hamlet
// fallback chain and country_code upper-casing.
func TestNominatimReverse_CityFallback(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCity string
		wantCC   string
	}{
		{
			name:     "city set",
			body:     `{"address":{"city":"Vienna","state":"Wien","country":"Austria","country_code":"at"}}`,
			wantCity: "Vienna",
			wantCC:   "AT",
		},
		{
			name:     "falls through city→town",
			body:     `{"address":{"town":"Innsbruck","state":"Tyrol","country":"Austria","country_code":"at"}}`,
			wantCity: "Innsbruck",
			wantCC:   "AT",
		},
		{
			name:     "falls through to village",
			body:     `{"address":{"village":"Matrei","state":"Tyrol","country":"Austria","country_code":"at"}}`,
			wantCity: "Matrei",
			wantCC:   "AT",
		},
		{
			name:     "falls through to hamlet",
			body:     `{"address":{"hamlet":"Tiny","country":"Austria","country_code":"at"}}`,
			wantCity: "Tiny",
			wantCC:   "AT",
		},
		{
			name:     "city wins over town",
			body:     `{"address":{"city":"Big","town":"Small","country_code":"de"}}`,
			wantCity: "Big",
			wantCC:   "DE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("User-Agent"); !strings.Contains(got, "Skadia") {
					t.Errorf("missing Skadia User-Agent, got %q", got)
				}
				// Verify params rounded to 3 decimals: we expect the fractional digit count <= 3.
				if lat := r.URL.Query().Get("lat"); strings.Count(lat, ".") == 1 {
					frac := lat[strings.Index(lat, ".")+1:]
					if len(frac) > 3 {
						t.Errorf("lat not rounded to 3 decimals: %q", lat)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			// httptest.NewServer binds to 127.0.0.1, which the SSRF validator
			// rejects by default. Opt in explicitly for these provider-shape
			// tests — production admins use the same opt-in for in-cluster
			// Photon/Nominatim mirrors.
			bypassValidationForTest(t)

			n := &Nominatim{BaseURL: srv.URL, Timeout: 2 * time.Second}
			// Loosen the process-wide 1-req/s bucket so consecutive
			// subtests don't serialize on the limiter. withRelaxedLimiters
			// restores the production default on t.Cleanup — critical for
			// test isolation (without the restore, a subsequent test that
			// relies on burst=1 would see a leaked wide bucket).
			withRelaxedLimiters(t)
			got, err := n.Reverse(context.Background(), 47.26891234, 11.3931112, "de")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatalf("result is nil")
			}
			if got.City != tc.wantCity {
				t.Errorf("city = %q, want %q", got.City, tc.wantCity)
			}
			if got.CountryCode != tc.wantCC {
				t.Errorf("country_code = %q, want %q", got.CountryCode, tc.wantCC)
			}
		})
	}
}

func TestNominatimReverse_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"address":{}}`))
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	n := &Nominatim{BaseURL: srv.URL, Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	got, err := n.Reverse(context.Background(), 0, 0, "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result for empty address, got %+v", got)
	}
}

// TestNominatimReverse_RejectsPrivateBaseURL is the integration-level SSRF
// guard: when a user has configured a localhost BaseURL, the Reverse() call
// must refuse to dispatch. This is the finding #10 mitigation — no escape
// hatch exists, so the validator always rejects.
func TestNominatimReverse_RejectsPrivateBaseURL(t *testing.T) {
	n := &Nominatim{BaseURL: "http://127.0.0.1:5432/reverse", Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	_, err := n.Reverse(context.Background(), 1, 1, "de")
	if err == nil {
		t.Fatal("expected SSRF-rejection error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid geocoding base URL") {
		t.Errorf("error %q should mention invalid base URL", err.Error())
	}
}

func TestNominatimReverse_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	n := &Nominatim{BaseURL: srv.URL, Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	_, err := n.Reverse(context.Background(), 1, 1, "de")
	if err == nil {
		t.Fatal("expected error on 429, got nil")
	}
}

// TestNominatimReverse_ErrorKeyBody verifies finding #46: Nominatim signals
// "invalid coordinates" / "Unable to geocode" with HTTP 200 + {"error":"…"}.
// The adapter must treat this as a soft miss (return nil, nil) rather than
// parsing the empty address block as a "success with all fields empty".
func TestNominatimReverse_ErrorKeyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Real payload shape reported by Nominatim for rejected coords.
		_, _ = w.Write([]byte(`{"error":"Unable to geocode"}`))
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	n := &Nominatim{BaseURL: srv.URL, Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	got, err := n.Reverse(context.Background(), 1, 1, "de")
	if err != nil {
		t.Fatalf("unexpected error for error-body response: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil result for error body, got %+v", got)
	}
}

// TestNominatimReverse_BaseURLWithFragment verifies finding #9: a base URL
// carrying a fragment must still produce a correctly-formed request URL
// (query string BEFORE the fragment, not after). The test server records
// the parsed query and asserts the core params arrived intact.
func TestNominatimReverse_BaseURLWithFragment(t *testing.T) {
	var receivedQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"address":{"city":"Vienna","country_code":"at"}}`))
	}))
	defer srv.Close()

	bypassValidationForTest(t)

	// Admin-pasted URL including a fragment — previously the naive concat
	// produced "…/reverse#frag?lat=…" which servers dropped entirely.
	n := &Nominatim{BaseURL: srv.URL + "/reverse#section", Timeout: 2 * time.Second}
	withRelaxedLimiters(t)
	got, err := n.Reverse(context.Background(), 48.2, 16.37, "de")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.City != "Vienna" {
		t.Fatalf("result = %+v, want City=Vienna", got)
	}
	if receivedQuery.Get("lat") == "" || receivedQuery.Get("lon") == "" {
		t.Errorf("server did not receive lat/lon params: %v", receivedQuery)
	}
	if receivedQuery.Get("format") != "jsonv2" {
		t.Errorf("format param missing or wrong: %q", receivedQuery.Get("format"))
	}
}
