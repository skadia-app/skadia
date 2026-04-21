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
// h.RouteCalculate and returns the recorder. A synthetic user-id is injected
// via context because RouteCalculate is auth-gated (mounted inside the JWT
// middleware group in main.go).
func postRouteCalculate(h *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/routing/route-internal",
		strings.NewReader(body))
	req = req.WithContext(contextWithTestUserID(req.Context(), "00000000-0000-0000-0000-000000000001"))
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
// per variant, and that both endpoints emit values in the [0.0, 1.0] scale
// (V6-Retrofit parity gate — also guards against scale-mismatch bugs like the
// 0–100 vs 0.0–1.0 regression in aggregateForestProtectedFromResult).
//
// Shape-Asymmetrie (bindend):
//
//	/routes/suggest → body.suggestions[0].{forest_pct, …}             (Top-Level struct)
//	/routing/route-internal → body.features[0].properties.{forest_pct, …}
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
	// Route into the forest-tagged bottom-right quadrant so forest_pct > 0.
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

	// Value-scale parity: both endpoints must emit forest_pct and protected_pct
	// in [0.0, 1.0]. This guards against scale-mismatch regressions where
	// aggregateForestProtectedFromResult returns 0–100 raw values instead of
	// the 0.0–1.0 scale that RouteSuggestion.forest_pct uses.
	scalarFields := []string{"forest_pct", "protected_pct"}
	for _, key := range scalarFields {
		// /routing/route-internal: value is float64 after JSON decode.
		if v, ok := calcProps[key]; ok {
			fv, ok2 := v.(float64)
			if !ok2 {
				t.Errorf("/routing/route-internal %s: expected float64, got %T", key, v)
				continue
			}
			if fv < 0 || fv > 1 {
				t.Errorf("/routing/route-internal %s out of [0,1]: %v (scale bug?)", key, fv)
			}
		}
		// /routes/suggest: value is float64 after re-encode/decode of float32 struct.
		if v, ok := suggMap[key]; ok {
			fv, ok2 := v.(float64)
			if !ok2 {
				t.Errorf("/routes/suggest %s: expected float64, got %T", key, v)
				continue
			}
			if fv < 0 || fv > 1 {
				t.Errorf("/routes/suggest %s out of [0,1]: %v (scale bug?)", key, fv)
			}
		}
	}

	// forest_pct must be > 0 for the route-internal call because the fixture's
	// bottom-right quadrant edges are tagged with ForestPct=70. This ensures the
	// test exercises a non-trivial code path (not just a zero-value pass-through).
	calcForestPct, _ := calcProps["forest_pct"].(float64)
	if calcForestPct <= 0 {
		t.Errorf("/routing/route-internal forest_pct: expected > 0 for forest-quadrant route, got %v", calcForestPct)
	}
}

// TestRouteCalculateEnrichmentRangeKeyParity verifies that when both endpoints
// produce range data (because the route traverses tagged edges), both carry the
// same set of content-dependent range keys.
//
// Strategy: use the bottom-right forest quadrant fixture (ForestPct=70 on edges
// r≥2, c≥2).  A route from (48.990, 12.100) to (48.954, 12.154) traverses those
// edges — so forest_ranges MUST appear in both responses.  protected_ranges is
// absent in the fixture (protected_area==0) and MUST be absent in both.  This
// tests the omit-when-empty symmetry.
//
// summits_passed: requires h.DB != nil — absent in both (h.DB=nil fixture).
// The test verifies that neither response contains summits_passed, confirming
// the nil-DB guard is symmetric.
//
// sac_ranges / mtb_ranges / vf_ranges: not present in the base fixture (no
// SAC/MTB/VF tags).  Absent in both — symmetric omit-when-empty confirmed by
// negative assertion.
func TestRouteCalculateEnrichmentRangeKeyParity(t *testing.T) {
	setup := newTestRoutingHandlerWithCoverage(t)

	// Route through the forest-quadrant (r≥2, c≥2) edges: ForestPct=70 encoded.
	const routeBody = `{"from":[48.990,12.100],"to":[48.954,12.154]}`

	// --- /routing/route-internal ---
	calcW := postRouteCalculate(setup.Handler, routeBody)
	if calcW.Code != http.StatusOK {
		t.Fatalf("/routing/route-internal: expected 200, got %d: %s", calcW.Code, calcW.Body.String())
	}
	var calcResp map[string]any
	if err := json.Unmarshal(calcW.Body.Bytes(), &calcResp); err != nil {
		t.Fatalf("unmarshal calc: %v", err)
	}
	calcFeatures := calcResp["features"].([]any)
	calcProps := calcFeatures[0].(map[string]any)["properties"].(map[string]any)

	// forest_ranges: MUST be present (ForestPct=70 ≥ threshold 60).
	if _, ok := calcProps["forest_ranges"]; !ok {
		t.Errorf("/routing/route-internal: forest_ranges missing (expected for forest-quadrant route)")
	}
	// protected_ranges: MUST be absent (protected_area=0 < threshold 1).
	if _, ok := calcProps["protected_ranges"]; ok {
		t.Errorf("/routing/route-internal: protected_ranges present but protected_area=0 in fixture")
	}
	// sac_ranges / mtb_ranges / vf_ranges: no tags in base fixture → absent.
	for _, key := range []string{"sac_ranges", "mtb_ranges", "vf_ranges", "vf_unknown_ranges"} {
		if _, ok := calcProps[key]; ok {
			t.Errorf("/routing/route-internal: %s present but fixture has no outdoor tags", key)
		}
	}
	// summits_passed: h.DB=nil → absent.
	if _, ok := calcProps["summits_passed"]; ok {
		t.Errorf("/routing/route-internal: summits_passed present but h.DB=nil fixture")
	}

	// forest_ranges shape: must be a non-empty array of objects with "from"/"to".
	if frRaw, ok := calcProps["forest_ranges"]; ok {
		fr, ok := frRaw.([]any)
		if !ok || len(fr) == 0 {
			t.Errorf("/routing/route-internal: forest_ranges is not a non-empty array: %T %v", frRaw, frRaw)
		} else {
			for j, elem := range fr {
				obj, ok := elem.(map[string]any)
				if !ok {
					t.Errorf("/routing/route-internal: forest_ranges[%d] is not an object", j)
					continue
				}
				if _, ok := obj["from"]; !ok {
					t.Errorf("/routing/route-internal: forest_ranges[%d] missing 'from'", j)
				}
				if _, ok := obj["to"]; !ok {
					t.Errorf("/routing/route-internal: forest_ranges[%d] missing 'to'", j)
				}
			}
		}
	}

	// --- /routes/suggest ---
	// Use a start that leads into the forest quadrant.
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
	// Re-encode first suggestion to a map for key inspection.
	suggJSON, _ := json.Marshal(suggestResp.Suggestions[0])
	var suggMap map[string]any
	_ = json.Unmarshal(suggJSON, &suggMap)

	// sac_ranges / mtb_ranges / vf_ranges: no outdoor tags in base fixture → absent.
	for _, key := range []string{"sac_ranges", "mtb_ranges", "vf_ranges", "vf_unknown_ranges"} {
		if _, ok := suggMap[key]; ok {
			t.Errorf("/routes/suggest: %s present but fixture has no outdoor tags", key)
		}
	}
	// summits_passed: h.DB=nil → absent (omitempty on RouteSuggestion.SummitsPassed).
	if _, ok := suggMap["summits_passed"]; ok {
		t.Errorf("/routes/suggest: summits_passed present but h.DB=nil fixture")
	}
	// protected_ranges: absent (protected_area=0).
	if _, ok := suggMap["protected_ranges"]; ok {
		t.Errorf("/routes/suggest: protected_ranges present but protected_area=0 in fixture")
	}
}
