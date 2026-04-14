package geocoding

import (
	"net/url"
	"strings"
	"testing"
)

// TestBuildProviderURL_Fragment verifies finding #9: base URLs carrying a
// #fragment must still produce a request URL where the query string sits
// BEFORE the fragment, not after it. The previous naive
// base+"?"+q.Encode() implementation would emit "…#frag?lat=…" — servers
// treat the fragment as client-side only and never saw the query.
func TestBuildProviderURL_Fragment(t *testing.T) {
	extra := url.Values{}
	extra.Set("lat", "1.5")
	extra.Set("lon", "2.5")

	got, err := buildProviderURL("https://example.com/reverse#section", extra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("resulting URL not parseable: %v", err)
	}
	if u.Fragment != "section" {
		t.Errorf("fragment = %q, want section", u.Fragment)
	}
	if u.Query().Get("lat") != "1.5" || u.Query().Get("lon") != "2.5" {
		t.Errorf("query lost: %+v", u.Query())
	}
	// Serialized output must have '?' before '#', not after.
	qIdx := strings.Index(got, "?")
	hIdx := strings.Index(got, "#")
	if qIdx == -1 || hIdx == -1 || qIdx > hIdx {
		t.Errorf("serialized URL %q has fragment before query", got)
	}
}

// TestBuildProviderURL_MergesExistingQuery covers the "admin pastes a base
// URL with an api_key or similar query parameter" case. buildProviderURL
// must PRESERVE those and layer its own params on top.
func TestBuildProviderURL_MergesExistingQuery(t *testing.T) {
	extra := url.Values{}
	extra.Set("lat", "10")
	extra.Set("format", "jsonv2")

	got, err := buildProviderURL("https://example.com/reverse?api_key=sekret&format=xml", extra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("resulting URL not parseable: %v", err)
	}
	q := u.Query()
	if q.Get("api_key") != "sekret" {
		t.Errorf("api_key stripped: %q", q.Get("api_key"))
	}
	// Our extra must override the pre-existing format=xml.
	if q.Get("format") != "jsonv2" {
		t.Errorf("format = %q, want jsonv2 (extra must win)", q.Get("format"))
	}
	if q.Get("lat") != "10" {
		t.Errorf("lat missing: %+v", q)
	}
}

// TestBuildProviderURL_NoBaseQuery covers the simplest shape (default
// public endpoint with no pre-existing query or fragment).
func TestBuildProviderURL_NoBaseQuery(t *testing.T) {
	extra := url.Values{}
	extra.Set("lat", "47.2")

	got, err := buildProviderURL("https://example.com/reverse", extra)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.com/reverse?lat=47.2" {
		t.Errorf("got %q, want https://example.com/reverse?lat=47.2", got)
	}
}

// TestBuildProviderURL_ParseError surfaces parse failures for garbage
// input — we don't want a silent empty URL if someone manages to store a
// malformed base (e.g. via a manual DB edit that skipped ValidateBaseURL).
func TestBuildProviderURL_ParseError(t *testing.T) {
	_, err := buildProviderURL("http://exa\x00mple.com/", url.Values{})
	if err == nil {
		t.Fatal("expected parse error for control-byte host, got nil")
	}
}
