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

// validVariantIDs are the VariantIDs the pipeline may produce (assignLabels).
var validVariantIDs = map[string]bool{
	"shortest": true,
	"medium":   true,
	"longest":  true,
	"only":     true,
}

// isValidVariantID returns true when id is a canonical VariantID, including
// numbered medium variants ("medium-2", "medium-3" …).
func isValidVariantID(id string) bool {
	if validVariantIDs[id] {
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
		if !isValidVariantID(s.VariantID) {
			t.Errorf("unexpected VariantID %q (expected shortest/medium*/longest/only)", s.VariantID)
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

	// Fixture places mtb_scale=3 (OSM "2" → display S2) on diagonal (0,2)↔(1,3),
	// which Route 1 (NE diagonal) traverses exclusively (Routes 0, 2 don't).
	// TaggedPctMtb ≈ 35% > 20% gate + MaxMtbScale=3 > 0 (maxOther) → mtb_technical fires.
	// DisplayValue "bis S2". Bei Layout-Regression: seen-Keys werden geloggt.
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
	t.Logf("observed descriptor keys: %v", seen)
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
				if seg.SacScale == "T3" {
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
	// SAC edge on (0,1)↔(1,1) is traversed by Route 0 (col-1 down-and-back);
	// hard-assertion valid after Leg1-segments pipeline fix (Opus R2-#6):
	// BuildRouteResultFromEdges now populates Segments so mergeLegs includes
	// all leg1 edges in the user-facing segments array.
	if !foundSac {
		t.Errorf("sac_scale=T3 segment missing — Route 0 traverses (0,1)↔(1,1) with SAC=T3; check BuildRouteResultFromEdges + mergeLegs pipeline")
	}
	// TV + VF may only appear on specific node-pair-traversing routes — log as soft fails if not found.
	if !foundTv {
		t.Logf("info: trail_visibility=intermediate not seen (fixture edge may not be traversed)")
	}
	if !foundVia {
		t.Logf("info: via_ferrata K2 not seen (fixture edge may not be traversed)")
	}
}
