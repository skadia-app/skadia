package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// postRouteCalculate fires a POST /routing/route-internal request against
// h.RouteCalculate and returns the recorder. No auth context is injected
// because RouteCalculate is not auth-gated (L-12).
func postRouteCalculate(h *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/routing/route-internal",
		strings.NewReader(body))
	w := httptest.NewRecorder()
	h.RouteCalculate(w, req)
	return w
}

// filterEnrichmentKeys liefert die Keys aus props, die in expected sind, als
// sortierte Liste. Unbekannte Keys werden ignoriert — der Test prueft nur die
// Paritaet der expected-Subset.
func filterEnrichmentKeys(props map[string]any, expected map[string]bool) []string {
	var out []string
	for k := range props {
		if expected[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// TestRouteCalculateEnrichmentKeysPresent verifies that /routing/route-internal
// injects all expected enrichment keys into features[0].properties (V6-Retrofit
// Gate 10 — key presence, no DB required).
//
// The test uses the 5×5 grid fixture from testhelper_routing_test.go which has
// forest_pct=70 encoded in the bottom-right quadrant edges. h.DB is nil so
// summit detection is skipped (guarded in RouteCalculate), but all in-memory
// enrichment fields must still be present.
func TestRouteCalculateEnrichmentKeysPresent(t *testing.T) {
	setup := newTestRoutingHandlerWithCoverage(t)

	// Route from (48.990, 12.100) to (48.954, 12.154) — diagonal traversal of
	// the grid, hits the bottom-right forest quadrant (r≥2, c≥2).
	const body = `{"from":[48.990,12.100],"to":[48.954,12.154]}`
	w := postRouteCalculate(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/geo+json" {
		t.Errorf("expected Content-Type application/geo+json, got %q", ct)
	}

	var body2 map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body2); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, w.Body.String())
	}

	featuresRaw, ok := body2["features"]
	if !ok {
		t.Fatalf("no 'features' key in response: %s", w.Body.String())
	}
	features, ok := featuresRaw.([]any)
	if !ok || len(features) == 0 {
		t.Fatalf("features is empty or wrong type")
	}
	feat0, ok := features[0].(map[string]any)
	if !ok {
		t.Fatal("features[0] is not a map")
	}
	propsRaw, ok := feat0["properties"]
	if !ok {
		t.Fatal("no 'properties' in features[0]")
	}
	props, ok := propsRaw.(map[string]any)
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Enrichment keys that must always be present regardless of route content or DB.
	// Range keys (sac_ranges, mtb_ranges, vf_ranges, vf_unknown_ranges,
	// forest_ranges, protected_ranges) are omitted when no matching edges exist
	// — they are not tested here. summits_passed requires h.DB != nil.
	requiredKeys := []string{
		"forest_pct",
		"protected_pct",
	}

	for _, k := range requiredKeys {
		if _, exists := props[k]; !exists {
			t.Errorf("missing enrichment key %q in features[0].properties", k)
		}
	}
}

// TestRouteCalculateEnrichmentForestPct verifies that forest_pct is non-zero
// when the route traverses the forest-tagged bottom-right quadrant (fp=70).
func TestRouteCalculateEnrichmentForestPct(t *testing.T) {
	setup := newTestRoutingHandlerWithCoverage(t)

	// Route into the bottom-right forest quadrant.
	const body = `{"from":[48.990,12.100],"to":[48.954,12.154]}`
	w := postRouteCalculate(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	features := resp["features"].([]any)
	props := features[0].(map[string]any)["properties"].(map[string]any)

	fpRaw, ok := props["forest_pct"]
	if !ok {
		t.Fatal("forest_pct not present")
	}
	fp, ok := fpRaw.(float64)
	if !ok {
		t.Fatalf("forest_pct is not float64: %T", fpRaw)
	}
	// The forest quadrant has fp=70 — any route traversing those edges should
	// produce a non-zero distance-weighted forest_pct.
	if fp <= 0 {
		t.Errorf("expected forest_pct > 0 for route through forest quadrant, got %v", fp)
	}
}

// TestRouteCalculateEnrichmentKeyParity verifies that /routing/route-internal
// carries the same always-present enrichment keys that /routes/suggest carries
// per variant (V6-Retrofit parity gate).
//
// Shape-Asymmetrie (bindend):
//   /routes/suggest → body.suggestions[0].{forest_pct, …}             (Top-Level struct)
//   /routing/route-internal → body.features[0].properties.{forest_pct, …}
//
// Only the unconditionally-present scalar fields are compared here:
// - forest_pct and protected_pct are always in both responses.
// - Range keys (sac_ranges, mtb_ranges, etc.) use omitempty on the suggest side
//   and conditional insertion on the route-internal side, so their presence
//   depends on route content and cannot be compared with a content-free fixture.
// - summits_passed requires a DB connection and is absent in both (h.DB=nil fixture).
func TestRouteCalculateEnrichmentKeyParity(t *testing.T) {
	setup := newTestRoutingHandlerWithCoverage(t)

	// Always-present scalar enrichment keys (no omitempty, no DB dependency).
	alwaysPresentKeys := map[string]bool{
		"forest_pct":    true,
		"protected_pct": true,
	}

	// --- /routing/route-internal ---
	calcBody := `{"from":[48.990,12.100],"to":[48.954,12.154]}`
	calcW := postRouteCalculate(setup.Handler, calcBody)
	if calcW.Code != http.StatusOK {
		t.Fatalf("/routing/route-internal: expected 200, got %d: %s", calcW.Code, calcW.Body.String())
	}
	var calcResp map[string]any
	if err := json.Unmarshal(calcW.Body.Bytes(), &calcResp); err != nil {
		t.Fatalf("unmarshal calc: %v", err)
	}
	calcFeatures := calcResp["features"].([]any)
	calcProps := calcFeatures[0].(map[string]any)["properties"].(map[string]any)
	calcKeys := filterEnrichmentKeys(calcProps, alwaysPresentKeys)

	// --- /routes/suggest ---
	// SuggestRoutes requires a userID. Inject a test user ID via context.
	suggestBody := `{"start":[12.10,48.99],"target_distance_m":8000,"profile":"hiking"}`
	suggestReq := httptest.NewRequest(http.MethodPost, "/api/v1/routes/suggest",
		strings.NewReader(suggestBody))
	suggestReq = suggestReq.WithContext(contextWithTestUserID(suggestReq.Context(), "00000000-0000-0000-0000-000000000001"))
	suggestW := httptest.NewRecorder()
	setup.Handler.SuggestRoutes(suggestW, suggestReq)

	if suggestW.Code != http.StatusOK {
		t.Fatalf("/routes/suggest: expected 200, got %d: %s", suggestW.Code, suggestW.Body.String())
	}
	var suggestResp RouteSuggestionResponse
	if err := json.Unmarshal(suggestW.Body.Bytes(), &suggestResp); err != nil {
		t.Fatalf("unmarshal suggest: %v", err)
	}
	if len(suggestResp.Suggestions) == 0 {
		t.Fatal("/routes/suggest: expected at least one suggestion")
	}
	// Suggestions are structs — extract field names via re-encoding to map.
	// float32 fields that are zero still serialize (no omitempty).
	suggJSON, _ := json.Marshal(suggestResp.Suggestions[0])
	var suggMap map[string]any
	_ = json.Unmarshal(suggJSON, &suggMap)
	suggestKeys := filterEnrichmentKeys(suggMap, alwaysPresentKeys)

	// Both endpoints must carry all always-present enrichment keys.
	if len(calcKeys) != len(suggestKeys) {
		t.Errorf("enrichment key count diverges:\n  /routing/route-internal: %v\n  /routes/suggest: %v",
			calcKeys, suggestKeys)
		return
	}
	for i := range calcKeys {
		if calcKeys[i] != suggestKeys[i] {
			t.Errorf("enrichment keys diverge:\n  /routing/route-internal: %v\n  /routes/suggest: %v",
				calcKeys, suggestKeys)
			return
		}
	}
}
