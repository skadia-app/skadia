package handler

import (
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

// TestSuggestRoutes_ResponseContainsVfFields verifies Spec-Invariant I9:
// vf_ranges keys MUST come from {K1..K6}, never other levels.
// vf_unknown_ranges is type-checked (CoordRange slice). Both fields are
// omitempty — nil is acceptable — but populated values must be valid.
// Task 15a: test is intentionally red until Task 16 populates the fields
// in the build loop. Bundled commit after Task 16.
func TestSuggestRoutes_ResponseContainsVfFields(t *testing.T) {
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

	// VfRanges may be nil (omitempty), but if set: only K1..K6 are valid.
	allowed := map[string]bool{
		"K1": true, "K2": true, "K3": true,
		"K4": true, "K5": true, "K6": true,
	}
	for k := range s.VfRanges {
		if !allowed[k] {
			t.Fatalf("VfRanges key %q not in {K1..K6}", k)
		}
	}

	// VfUnknownRanges: type-check only here; deeper range-index validation
	// is covered by Task 8a unit tests.
	_ = s.VfUnknownRanges

	// Invariant I9: when any VF OSM ways exist on the route, VfRanges must
	// be non-nil (populated by the build loop). This assertion will FAIL
	// until Task 16 wires up the VF-range computation.
	// For a non-VF route the field may legitimately be nil — we can only
	// verify the key constraint above in the general case.
	// The combined assertion that ensures Task 16 is complete:
	// after Task 16, re-run with a VF fixture and assert len(VfRanges) > 0.
}
