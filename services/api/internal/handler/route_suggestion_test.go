package handler

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestSuggestRoutes_ResponseContainsForestProtectedPct verifies that
// RouteSuggestion carries ForestPct and ProtectedPct fields and that both
// values are within the expected [0, 1] range.
func TestSuggestRoutes_ResponseContainsForestProtectedPct(t *testing.T) {
	setup := newTestRoutingHandler(t)

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)
	if len(resp.Suggestions) == 0 {
		t.Skip("no suggestions")
	}
	s := resp.Suggestions[0]
	if s.ForestPct < 0 || s.ForestPct > 1 {
		t.Fatalf("ForestPct out of [0,1]: %v", s.ForestPct)
	}
	if s.ProtectedPct < 0 || s.ProtectedPct > 1 {
		t.Fatalf("ProtectedPct out of [0,1]: %v", s.ProtectedPct)
	}
}

// TestRouteSuggestion_VfRanges_KeyContract verifies Spec-Invariant I9 without
// a live routing pipeline: constructs a RouteSuggestion literal with known
// VfRanges and asserts that JSON round-trip preserves only K1..K6 keys.
//
// This is a pure contract/struct test — no DB, no routing graph needed.
// It replaces the vacuous HTTP-based check (which iterated over a nil map
// and always passed regardless of the implementation).
func TestRouteSuggestion_VfRanges_KeyContract(t *testing.T) {
	allowed := map[string]bool{
		"K1": true, "K2": true, "K3": true,
		"K4": true, "K5": true, "K6": true,
	}

	// Build a RouteSuggestion with every valid VF level populated.
	s := RouteSuggestion{
		Variant:     1,
		VariantID:   "medium",
		Descriptors: []VariantDescriptor{},
		VfRanges: map[string][]CoordRange{
			"K1": {{From: 0, To: 3}},
			"K3": {{From: 5, To: 9}},
			"K6": {{From: 12, To: 15}},
		},
		VfUnknownRanges: []CoordRange{{From: 20, To: 22}},
	}

	// Round-trip through JSON (verifies omitempty and key serialisation).
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal RouteSuggestion: %v", err)
	}
	var decoded RouteSuggestion
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("json.Unmarshal RouteSuggestion: %v", err)
	}

	// I9: every key in VfRanges must be in {K1..K6}.
	if len(decoded.VfRanges) == 0 {
		t.Fatal("VfRanges must be non-empty after round-trip with populated fixture")
	}
	for k := range decoded.VfRanges {
		if !allowed[k] {
			t.Errorf("VfRanges key %q not in {K1..K6}", k)
		}
	}

	// Verify all three levels survived the round-trip.
	for _, wantKey := range []string{"K1", "K3", "K6"} {
		if _, ok := decoded.VfRanges[wantKey]; !ok {
			t.Errorf("VfRanges missing key %q after round-trip", wantKey)
		}
	}

	// VfUnknownRanges: must survive round-trip with correct length.
	if len(decoded.VfUnknownRanges) != 1 {
		t.Errorf("VfUnknownRanges: want 1 range, got %d", len(decoded.VfUnknownRanges))
	}
}

// TestSuggestRoutes_VfRanges_KeysValidWhenPresent verifies Invariant I9 via
// the HTTP handler: if the routing fixture produces VfRanges, their keys must
// be in {K1..K6}. The test fixture has no VF-tagged edges (see
// testhelper_routing_test.go), so VfRanges is legitimately nil — the test
// exercises the nil-is-valid path without triggering a false failure.
//
// For verification that VfRanges IS populated on a real VF route, see
// route_suggestion_integration_test.go (//go:build integration).
func TestSuggestRoutes_VfRanges_KeysValidWhenPresent(t *testing.T) {
	setup := newTestRoutingHandler(t)

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)
	if len(resp.Suggestions) == 0 {
		t.Skip("no suggestions returned by test fixture")
	}

	allowed := map[string]bool{
		"K1": true, "K2": true, "K3": true,
		"K4": true, "K5": true, "K6": true,
	}
	for i, s := range resp.Suggestions {
		for k := range s.VfRanges {
			if !allowed[k] {
				t.Errorf("suggestion[%d]: VfRanges key %q not in {K1..K6}", i, k)
			}
		}
		// VfUnknownRanges: type presence only (no further assertion needed here;
		// deeper CoordRange index validation is covered by Task 8a unit tests).
		_ = s.VfUnknownRanges
	}
}
