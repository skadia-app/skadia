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
