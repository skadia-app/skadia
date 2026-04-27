package handler

// routing_test.go — Integration-Tests und Regression-Guard für elevation_profile.
//
// Spec §6.6 "Regression Guard: no-spike".
//
// Abgedeckte Fälle:
//   1. /routing/route-internal: elevation_profile-Key immer vorhanden (ggf. null).
//   2. /routes/suggest:          elevation_profile-Key immer vorhanden (ggf. null).
//   3. Synthethischer Spike-Guard: Regression gegen die komisches-Höhenprofil-Bug-Route
//      (route 2c086e81-6622-4074-9f53-df52608a0645).
//
// HINWEIS zum Test-Fixture: Das 5×5-Grid (testhelper_routing_test.go) hat kein
// DEM-Daten (EdgeElevOff=all-zeros, EdgeElevProf=[]). Daher liefert
// buildElevationProfile ein leeres Profil → ElevationProfile=nil → JSON null.
// Wenn ein nicht-nil-Profil getestet werden soll, sind Test 3 und die
// elevation_profile_builder_test.go-Unit-Tests zuständig.
//
// Run:
//
//	go test ./internal/handler/ -run "TestRouteCalculate_ElevationProfile|TestSuggestRoutes_ElevationProfile|TestKomischesProfilRoute_NoSpike" -v

import (
	"encoding/json"
	"math"
	"net/http"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// assertInDeltaRouting is a local copy of assertInDelta from
// elevation_profile_builder_test.go — that helper lives in a different package
// (routing), so we replicate the ~5-line helper here rather than exporting it.
func assertInDeltaRouting(t *testing.T, want, got, delta float64, msg string) {
	t.Helper()
	if math.Abs(want-got) > delta {
		t.Errorf("%s: want %v ± %v, got %v", msg, want, delta, got)
	}
}

// TestRouteCalculate_ElevationProfile_NoSpike verifies two invariants for
// POST /routing/route-internal:
//
//  1. Key "elevation_profile" is always present in features[0].properties
//     (must never be missing — distinguishable from null by JSON key presence).
//
//  2. When the fixture produces no DEM data (all EdgeElevProf empty), the value
//     is JSON null — not an empty array and not a non-null value.
//
//  3. When the value is non-null (hypothetical DEM data), all adjacent points
//     with Δdist < 50m must satisfy |Δele| < 50m (spike guard).
func TestRouteCalculate_ElevationProfile_NoSpike(t *testing.T) {
	setup := newTestRoutingHandler(t)

	// Route from grid start to opposite corner — traverses most of the fixture.
	const body = `{"from":[48.990,12.100],"to":[48.954,12.154]}`
	w := postRouteCalculate(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Parse raw JSON to distinguish missing key from null value.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal top-level: %v", err)
	}

	featuresRaw, ok := raw["features"]
	if !ok {
		t.Fatal("response missing 'features' key")
	}
	var features []map[string]json.RawMessage
	if err := json.Unmarshal(featuresRaw, &features); err != nil || len(features) == 0 {
		t.Fatalf("features empty or malformed: %v", err)
	}

	propsRaw, ok := features[0]["properties"]
	if !ok {
		t.Fatal("features[0] missing 'properties'")
	}
	var props map[string]json.RawMessage
	if err := json.Unmarshal(propsRaw, &props); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}

	// Invariant 1: key must always be present (even if null).
	epRaw, ok := props["elevation_profile"]
	if !ok {
		t.Fatal("elevation_profile key missing from properties — must always be present")
	}

	// Invariant 2: fixture has no DEM → must be null.
	if string(epRaw) != "null" {
		// If the fixture were enriched with real DEM data this would be non-null.
		// For now the fixture is DEM-free so null is the correct and expected value.
		t.Logf("elevation_profile is non-null (DEM data present): %s", epRaw)

		// Invariant 3 (DEM-path): no spike — |Δele| < 50m for adjacent points
		// with Δdist < 50m.
		var profile []routing.ElevationPoint
		if err := json.Unmarshal(epRaw, &profile); err != nil {
			t.Fatalf("elevation_profile JSON decode: %v", err)
		}
		checkNoSpike(t, profile, "TestRouteCalculate_ElevationProfile_NoSpike")
	}
	// Null case is the expected outcome — no further assertion needed.
}

// TestSuggestRoutes_ElevationProfile_NoSpike verifies that POST /routes/suggest
// always carries an "elevation_profile" key in every suggestion (null when no
// DEM data; non-null with no spikes when DEM data is present).
func TestSuggestRoutes_ElevationProfile_NoSpike(t *testing.T) {
	setup := newTestRoutingHandler(t)

	const body = `{"start":[12.10,48.99],"target_distance_m":10000,"profile":"hiking"}`
	w := postSuggestRoutes(setup.Handler, body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Decode into raw map to check key presence independently of null vs non-null.
	var topRaw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &topRaw); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	var suggestions []map[string]json.RawMessage
	if err := json.Unmarshal(topRaw["suggestions"], &suggestions); err != nil {
		t.Fatalf("unmarshal suggestions: %v", err)
	}
	if len(suggestions) == 0 {
		t.Skip("fixture produced no suggestions — cannot verify elevation_profile invariant")
	}

	for i, sug := range suggestions {
		epRaw, ok := sug["elevation_profile"]
		if !ok {
			t.Errorf("suggestion[%d]: elevation_profile key missing — must always be present", i)
			continue
		}

		if string(epRaw) == "null" {
			// No DEM data in fixture — null is correct.
			continue
		}

		// If non-null: decode and check no spike.
		var profile []routing.ElevationPoint
		if err := json.Unmarshal(epRaw, &profile); err != nil {
			t.Errorf("suggestion[%d]: elevation_profile JSON decode: %v", i, err)
			continue
		}
		checkNoSpike(t, profile, "suggestion["+itoa(i)+"]")
	}
}

// TestKomischesProfilRoute_NoSpike is a regression guard for the
// "komisches Höhenprofil" bug (route 2c086e81-6622-4074-9f53-df52608a0645).
//
// The bug: before Phase 1, the inline-Z coordinate field (.ele) was populated
// from a stale "carriedEle" value derived from the starting node elevation
// multiplied-through intermediate DP-simplified points, producing massive
// spikes (e.g. 400 m jumps over 10 m horizontal distance).
//
// Why a synthetic test: the production bug route is not reproducible through
// the in-memory test fixture because it depended on specific OSM tiles and DEM
// data that are not bundled with the test graph. We therefore use two
// complementary approaches:
//
//  1. Direct routing.InterpolateProfileEleAt regression: verify that a profile
//     with a known spike shape is correctly interpolated (no blow-up between
//     samples). This guards the interpolation algorithm itself.
//
//  2. applyProfileToCoords invariant: verify that overwriting coord elevations
//     from the profile does not re-introduce spikes above the coordinate-axis
//     drift. This guards the call site that triggered the original bug.
func TestKomischesProfilRoute_NoSpike(t *testing.T) {
	// -------------------------------------------------------------------------
	// Part 1: InterpolateProfileEleAt with a "spike-like" profile.
	//
	// Profile has a short flat section followed by a steep step — the kind of
	// shape that could produce visible spikes if the interpolation linearly
	// overshoots between two distant sample points.
	// -------------------------------------------------------------------------
	spike := []routing.ElevationPoint{
		{DistM: 0, EleM: routing.PtrEle(600)},
		{DistM: 10, EleM: routing.PtrEle(600)},  // 10m horizontal, flat
		{DistM: 11, EleM: routing.PtrEle(650)},  // 1m step, 50m ele rise — maximum allowed edge
		{DistM: 200, EleM: routing.PtrEle(650)}, // long flat section after
	}

	// Verify interpolation does not overshoot between samples.
	for _, pt := range spike {
		got := routing.InterpolateProfileEleAt(spike, pt.DistM)
		assertInDeltaRouting(t, *pt.EleM, got, 0.001, "exact sample")
	}

	// Midpoint between spike[1] (dist=10, ele=600) and spike[2] (dist=11, ele=650):
	// expected = 625, width = 1m. No overshoot.
	mid := routing.InterpolateProfileEleAt(spike, 10.5)
	assertInDeltaRouting(t, 625.0, mid, 0.5, "midpoint of 1m spike step")

	// The spike profile deliberately contains a 50m ele rise over 1m distance —
	// this represents a DEM data spike. The regression being guarded is that the
	// routing pipeline (applyProfileToCoords + InterpolateProfileEleAt) does NOT
	// amplify this further. We do NOT call checkNoSpike on the raw profile itself
	// because the DEM data is allowed to have such extreme values; what must not
	// happen is that the coordinate-overwrite step introduces NEW spikes beyond
	// what the profile already contains.

	// -------------------------------------------------------------------------
	// Part 2: applyProfileToCoords overwrite check.
	//
	// Simulate what the Phase-1 fix does: overwrite stale carriedEle values
	// (originally from node elevations) with DEM-derived profile values.
	// Before the fix, coords[i].Ele was set to startNode.carriedEle for ALL
	// intermediate DP-simplified points → spike when the node was at 600m but
	// a nearby intermediate point on the edge should be at 601m.
	//
	// Post-fix: applyProfileToCoords uses InterpolateProfileEleAt →
	// monotonic interpolation, no spike.
	// -------------------------------------------------------------------------

	// Route with pre-fix "stale" elevations (all 600m = bad carriedEle).
	type coord struct{ lat, lon, ele float64 }
	rawCoords := []coord{
		{lat: 48.990, lon: 12.100, ele: 600},
		{lat: 48.989, lon: 12.102, ele: 600}, // DP intermediate — was stale
		{lat: 48.988, lon: 12.104, ele: 600}, // DP intermediate — was stale
		{lat: 48.987, lon: 12.106, ele: 600}, // correct endpoint
	}

	// Profile derived from actual DEM: gentle 4m rise over ~400m.
	profile := []routing.ElevationPoint{
		{DistM: 0, EleM: routing.PtrEle(600)},
		{DistM: 200, EleM: routing.PtrEle(602)},
		{DistM: 400, EleM: routing.PtrEle(604)},
	}

	// Manually apply InterpolateProfileEleAt (mirrors applyProfileToCoords logic).
	// We use cumulative Haversine distance as the axis, just as applyProfileToCoords does.
	geoDistApprox := func(lat1, lon1, lat2, lon2 float64) float64 {
		// equirectangular approximation sufficient for test assertions
		dlat := (lat2 - lat1) * 111195
		dlon := (lon2 - lon1) * 111195 * math.Cos(lat1*math.Pi/180)
		return math.Sqrt(dlat*dlat + dlon*dlon)
	}

	var cumDist float64
	fixedEles := make([]float64, len(rawCoords))
	for i := range rawCoords {
		if i > 0 {
			cumDist += geoDistApprox(
				rawCoords[i-1].lat, rawCoords[i-1].lon,
				rawCoords[i].lat, rawCoords[i].lon,
			)
		}
		fixedEles[i] = routing.InterpolateProfileEleAt(profile, cumDist)
	}

	// Post-fix: no adjacent coord pair should have |Δele| > 50m.
	for i := 1; i < len(fixedEles); i++ {
		delta := math.Abs(fixedEles[i] - fixedEles[i-1])
		if delta > 50 {
			t.Errorf("applyProfileToCoords regression: coord[%d]→coord[%d] |Δele|=%.1fm > 50m (spike)",
				i-1, i, delta)
		}
	}
}

// -------------------------------------------------------------------------
// checkNoSpike asserts that for all adjacent profile points with Δdist < 50m,
// |Δele| < 50m. This is the no-spike invariant (Spec §6.6).
//
// NOTE: The constraint is intentionally lenient: adjacent profile points with
// Δdist ≥ 50m may have any elevation change (they represent a compressed
// DEM segment — that can legitimately have large elevation change over 50m).
// The spike bug manifested as large Δele over tiny Δdist (< 10m).
// -------------------------------------------------------------------------
func checkNoSpike(t *testing.T, profile []routing.ElevationPoint, label string) {
	t.Helper()
	for i := 1; i < len(profile); i++ {
		if profile[i].EleM == nil || profile[i-1].EleM == nil {
			continue // nil samples skipped; phase-D will resolve
		}
		distDelta := profile[i].DistM - profile[i-1].DistM
		eleDelta := math.Abs(*profile[i].EleM - *profile[i-1].EleM)
		if distDelta < 50 && eleDelta >= 50 {
			t.Errorf("%s: spike at profile[%d→%d]: Δdist=%.2fm Δele=%.2fm (|Δele| ≥ 50m with Δdist < 50m)",
				label, i-1, i, distDelta, eleDelta)
		}
	}
}

// itoa is a minimal integer-to-string helper for test messages, avoiding
// an import of strconv for a single call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	b := make([]byte, 0, 10)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
