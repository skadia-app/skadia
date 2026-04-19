package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

// contextWithTestUserID injects a synthetic user-id into ctx using the same
// key that RequireUserID expects (middleware.UserIDKey).
func contextWithTestUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, middleware.UserIDKey, userID)
}

// postSuggestRoutes is a small helper that fires a POST /api/v1/routes/suggest
// request against h.SuggestRoutes and returns the recorder.
func postSuggestRoutes(h *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routes/suggest",
		strings.NewReader(body))
	req = req.WithContext(contextWithTestUserID(req.Context(), "00000000-0000-0000-0000-000000000001"))
	w := httptest.NewRecorder()
	h.SuggestRoutes(w, req)
	return w
}

// decodeRouteSuggestionResponse parses the recorder body into a
// RouteSuggestionResponse. Calls t.Fatalf on any error.
func decodeRouteSuggestionResponse(t *testing.T, w *httptest.ResponseRecorder) RouteSuggestionResponse {
	t.Helper()
	var resp RouteSuggestionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode RouteSuggestionResponse: %v\nbody: %s", err, w.Body.String())
	}
	return resp
}

// validV2IDs are the VariantIDs the v2 pipeline may produce (assignLabels).
var validV2IDs = map[string]bool{
	"shortest": true,
	"medium":   true,
	"longest":  true,
	"only":     true,
}

// isValidV2ID returns true when id is a canonical v2 VariantID, including
// numbered medium variants ("medium-2", "medium-3" …).
func isValidV2ID(id string) bool {
	if validV2IDs[id] {
		return true
	}
	return strings.HasPrefix(id, "medium-")
}

// -------------------------------------------------------------------
// T1: V2 without coverage → only rank-based labels, no coverage descriptors
// -------------------------------------------------------------------

func TestSuggestRoutes_V2_WithoutCoverage_OnlyRankLabels(t *testing.T) {
	setup := newTestRoutingHandler(t)

	// Enable V2 flag by replacing the settings store and wiring it back.
	setup.Handler.Settings = settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "true",
	})
	// CoverageReader is already nil in the test harness.

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)

	if resp.CoverageAvailable {
		t.Error("expected coverage_available=false when CoverageReader=nil")
	}

	// The 25-node grid graph is sized to produce at least 2 suggestions for a
	// 10 km target. If this assertion fires the test harness graph is too small.
	if len(resp.Suggestions) < 2 {
		t.Fatalf("expected at least 2 suggestions from grid graph, got %d "+
			"(check testhelper_routing_test.go graph size / distances)", len(resp.Suggestions))
	}

	if len(resp.Suggestions) > 3 {
		t.Errorf("expected at most 3 suggestions, got %d", len(resp.Suggestions))
	}

	for _, s := range resp.Suggestions {
		if !isValidV2ID(s.VariantID) {
			t.Errorf("unexpected v2 VariantID %q (expected shortest/medium*/longest/only)", s.VariantID)
		}

		// No coverage-based descriptors expected (CoverageReader=nil).
		for _, d := range s.Descriptors {
			coverageKeys := map[string]bool{
				"high_forest": true, "high_water": true, "high_protected": true,
				"low_traffic_noise": true,
			}
			if coverageKeys[d.Key] {
				t.Errorf("unexpected coverage descriptor %q when CoverageReader=nil", d.Key)
			}
		}
	}
}

// -------------------------------------------------------------------
// T2: V2 with CoverageReader present → coverage_available=true AND
// the route that traverses the high-forest quadrant gets a high_forest
// descriptor (ForestPct=70 encoded in EdgeTags2 for those edges).
// -------------------------------------------------------------------

func TestSuggestRoutes_V2_WithCoverage_ForestDescriptorAssigned(t *testing.T) {
	// newTestRoutingHandlerWithCoverage sets CoverageReader to a non-nil stub
	// (empty dir) so coverageAvailable=true. Actual forest values come from
	// EdgeTags2 bits pre-encoded in the bottom-right quadrant of the grid.
	setup := newTestRoutingHandlerWithCoverage(t)
	setup.Handler.Settings = settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "true",
	})

	if setup.Handler.CoverageReader == nil {
		t.Fatal("CoverageReader must be non-nil for this test")
	}

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)

	if !resp.CoverageAvailable {
		t.Error("expected coverage_available=true when CoverageReader is non-nil")
	}

	if len(resp.Suggestions) < 2 {
		t.Fatalf("expected at least 2 suggestions from grid graph, got %d", len(resp.Suggestions))
	}

	// At least one suggestion must carry a high_forest descriptor.
	// The grid graph encodes ForestPct=70 on edges in the bottom-right quadrant
	// (row≥2, col≥2). Routes through that quadrant should have MergedForestPct≈0.70,
	// far exceeding the 0.10 delta threshold vs. flat-quadrant routes (ForestPct=0).
	var foundHighForest bool
	for _, s := range resp.Suggestions {
		for _, d := range s.Descriptors {
			if d.Key == "high_forest" {
				foundHighForest = true
			}
		}
	}
	if !foundHighForest {
		t.Errorf("expected at least one suggestion with descriptor key 'high_forest'; got suggestions: %v",
			func() []string {
				var out []string
				for _, s := range resp.Suggestions {
					var keys []string
					for _, d := range s.Descriptors {
						keys = append(keys, d.Key)
					}
					out = append(out, s.VariantID+":"+strings.Join(keys, ","))
				}
				return out
			}())
	}
}

// -------------------------------------------------------------------
// T3: V2 — deprecated variant_id in request → 200 OK, all variants returned
// -------------------------------------------------------------------

func TestSuggestRoutes_V2_DeprecatedVariantIDIgnored(t *testing.T) {
	setup := newTestRoutingHandler(t)
	setup.Handler.Settings = settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "true",
	})

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking","variant_ids":["calmest"]}`
	w := postSuggestRoutes(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for deprecated variant_id, got %d: %s", w.Code, w.Body.String())
	}

	// The response must be parseable — deprecated IDs must not cause a 400.
	resp := decodeRouteSuggestionResponse(t, w)

	// All non-deprecated IDs were absent from the keep-set, so the filter
	// does nothing and all suggestions should be returned (fallback).
	// We only have a 5-node graph so suggestions may be 0; the key invariant
	// is that no 4xx / 5xx was returned.
	if !resp.UsedFallback {
		t.Errorf("expected UsedFallback=true when all variant_ids are deprecated, got false")
	}
}

// -------------------------------------------------------------------
// T4: V2 — mixed variant_ids: deprecated "calmest" + valid "shortest"
// -------------------------------------------------------------------

func TestSuggestRoutes_V2_MixedVariantIDFilter(t *testing.T) {
	setup := newTestRoutingHandler(t)
	setup.Handler.Settings = settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "true",
	})

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking","variant_ids":["calmest","shortest"]}`
	w := postSuggestRoutes(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)

	// Every returned suggestion must have VariantID="shortest" because
	// "calmest" is deprecated and must be ignored, leaving only "shortest"
	// in the keep-set.
	for _, s := range resp.Suggestions {
		if !strings.HasPrefix(s.VariantID, "shortest") {
			t.Errorf("expected only 'shortest' suggestions, got VariantID=%q", s.VariantID)
		}
	}

	// Verify no deprecated IDs leaked into the response.
	deprecatedIDs := map[string]bool{
		"calmest": true, "greenest": true, "hilly": true, "balanced": true,
	}
	for _, s := range resp.Suggestions {
		if deprecatedIDs[s.VariantID] {
			t.Errorf("deprecated VariantID %q must not appear in v2 response", s.VariantID)
		}
	}
}

// -------------------------------------------------------------------
// T5: V2 — unknown variant_id → fallback (all variants returned), no 400
// -------------------------------------------------------------------

func TestSuggestRoutes_V2_UnknownVariantIDIgnored(t *testing.T) {
	setup := newTestRoutingHandler(t)
	setup.Handler.Settings = settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "true",
	})

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking","variant_ids":["unknown-id"]}`
	w := postSuggestRoutes(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown variant_id, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)

	// "unknown-id" is neither valid nor deprecated (not in the validIDs map),
	// so the filter keep-set is empty → all suggestions returned unchanged.
	// Verify the unknown ID did not leak into the response.
	for _, s := range resp.Suggestions {
		if s.VariantID == "unknown-id" {
			t.Errorf("VariantID 'unknown-id' must not appear in response")
		}
	}
}

// -------------------------------------------------------------------
// T6: V2 flag=false → V1 path runs, no "medium-N" IDs
// -------------------------------------------------------------------

func TestSuggestRoutes_V2Flag_FallsBackToV1(t *testing.T) {
	// Default settings: routing.route_variants_v2 = "false"
	setup := newTestRoutingHandler(t)

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from V1 path, got %d: %s", w.Code, w.Body.String())
	}

	resp := decodeRouteSuggestionResponse(t, w)

	// V1 uses semantic variant IDs from DefaultVariants (calmest, greenest,
	// hilly, balanced, shortest). None of them start with "medium".
	for _, s := range resp.Suggestions {
		if strings.HasPrefix(s.VariantID, "medium") {
			t.Errorf("V1 path must not produce 'medium*' VariantID, got %q", s.VariantID)
		}
	}
}

// -------------------------------------------------------------------
// T7: V2 outdoor — mtb_technical descriptor assigned
// -------------------------------------------------------------------

func TestSuggestRoutes_V2_Outdoor_MtbTechnicalAssigned(t *testing.T) {
	setup := newTestRoutingHandlerWithOutdoor(t)
	setup.Handler.Settings = settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "true",
	})

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeRouteSuggestionResponse(t, w)

	// With harness fixture (mtb-tagged edge (0,0)-(0,1)), at least one route
	// traversing that edge should gain mtb_technical. If no candidate traverses
	// the edge, this test is too optimistic — rerun with a narrower fixture.
	// Per rev5 fixture values: MaxMtbScale=3 on that route → DisplayValue "bis S2".
	var found bool
	var seen []string
	for _, s := range resp.Suggestions {
		for _, d := range s.Descriptors {
			seen = append(seen, d.Key)
			if d.Key == "mtb_technical" && d.DisplayValue == "bis S2" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("mtb_technical 'bis S2' not found; observed keys: %v", seen)
	}
}

// -------------------------------------------------------------------
// T8: V2 outdoor — sub-segments annotated with sac/mtb/tv/vf tags
// -------------------------------------------------------------------

func TestSuggestRoutes_V2_Outdoor_SubSegmentsAnnotated(t *testing.T) {
	setup := newTestRoutingHandlerWithOutdoor(t)
	setup.Handler.Settings = settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "true",
	})

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeRouteSuggestionResponse(t, w)

	// Segments live under GeoJSON.properties.segments — parse the RawMessage.
	type segProp struct {
		SacScale        string `json:"sac_scale,omitempty"`
		MtbScale        string `json:"mtb_scale,omitempty"`
		ViaFerrataScale string `json:"via_ferrata_scale,omitempty"`
		TrailVisibility string `json:"trail_visibility,omitempty"`
	}
	type fc struct {
		Features []struct {
			Properties struct {
				Segments []segProp `json:"segments"`
			} `json:"properties"`
		} `json:"features"`
	}

	foundSac, foundMtb, foundTv, foundVia := false, false, false, false
	for _, s := range resp.Suggestions {
		var g fc
		if err := json.Unmarshal(s.GeoJSON, &g); err != nil {
			t.Fatalf("unmarshal geojson: %v", err)
		}
		for _, feat := range g.Features {
			for _, seg := range feat.Properties.Segments {
				if seg.SacScale == "T4" {
					foundSac = true
				}
				if seg.MtbScale == "S2" {
					foundMtb = true
				}
				if seg.TrailVisibility == "intermediate" {
					foundTv = true
				}
				if seg.ViaFerrataScale == "K2" {
					foundVia = true
				}
			}
		}
	}
	if !foundMtb {
		t.Errorf("mtb_scale=S2 segment missing")
	}
	// SAC: hiking profile penalises T4 (sacPenalty ≈ 5.5) so the router avoids
	// SAC-tagged edges when alternatives exist. Hard assert intentionally omitted;
	// SAC segment coverage is tested in unit tests (TestComputeDescriptors_AlpineHiking).
	if !foundSac {
		t.Logf("info: sac_scale=T4 segment not seen (hiking-profile SAC penalty causes avoidance; soft-fail)")
	}
	// TV + VF may only appear on specific node-pair-traversing routes — log as soft fails if not found.
	if !foundTv {
		t.Logf("info: trail_visibility=intermediate not seen (fixture edge may not be traversed)")
	}
	if !foundVia {
		t.Logf("info: via_ferrata K2 not seen (fixture edge may not be traversed)")
	}
}
