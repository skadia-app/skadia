package handler

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/dem"
)

// assertInDelta is the local stdlib-only float-equal helper (testify is in
// go.sum but not used in source). Mirrors the helper from
// services/api/internal/routing/elevation_profile_builder_test.go.
func assertInDelta(t *testing.T, want, got, delta float64, msg string) {
	t.Helper()
	if math.Abs(want-got) > delta {
		t.Errorf("%s: want %v ± %v, got %v", msg, want, delta, got)
	}
}

// bc builds a brouterCoord with a usable Z value.
func bc(lon, lat, ele float64) brouterCoord {
	return brouterCoord{Lon: lon, Lat: lat, Ele: ele, EleOK: true}
}

// bcNoZ builds a 2D coord with EleOK=false (e.g. JSON null mapped through).
func bcNoZ(lon, lat float64) brouterCoord {
	return brouterCoord{Lon: lon, Lat: lat, EleOK: false}
}

// ---- buildProfileFromInlineZ ----

func TestBuildProfileFromInlineZ_RealBrouterShape(t *testing.T) {
	coords := []brouterCoord{
		bc(11.404, 47.270, 581),
		bc(11.405, 47.271, 590),
		bc(11.406, 47.272, 600),
	}
	profile := buildProfileFromInlineZ(coords)
	if len(profile) != 3 {
		t.Fatalf("want 3 profile points, got %d", len(profile))
	}
	assertInDelta(t, 0.0, profile[0]["dist_m"].(float64), 0.01, "profile[0].dist_m")
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
	if !(profile[1]["dist_m"].(float64) > profile[0]["dist_m"].(float64)) {
		t.Errorf("profile[1].dist_m must be > profile[0].dist_m")
	}
	if !(profile[2]["dist_m"].(float64) > profile[1]["dist_m"].(float64)) {
		t.Errorf("profile[2].dist_m must be > profile[1].dist_m")
	}
}

func TestRouteViabrouterRejectsUnknownProfileParamBeforeProxy(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	h := &Handler{Config: &HandlerConfig{BRouterURL: server.URL}}
	req := httptest.NewRequest(http.MethodPost, "/routing/route", strings.NewReader(`{
		"waypoints": [[47.259,11.394],[47.280,11.450]],
		"profile": "hiking-mountain",
		"profile_params": {"unknown_param": "1"}
	}`))
	req = req.WithContext(contextWithTestUserID(req.Context(), "00000000-0000-0000-0000-000000000001"))
	w := httptest.NewRecorder()

	h.RouteViabrouter(w, req)

	if called {
		t.Fatal("unknown profile param should be rejected before calling BRouter")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestBuildProfileFromInlineZ_NoZ_ReturnsNil(t *testing.T) {
	coords := []brouterCoord{bcNoZ(11.404, 47.270), bcNoZ(11.405, 47.271)}
	if got := buildProfileFromInlineZ(coords); got != nil {
		t.Errorf("want nil for no-Z coords, got %v", got)
	}
}

func TestBuildProfileFromInlineZ_AllZeroZ_ReturnsNil(t *testing.T) {
	coords := []brouterCoord{bc(11.404, 47.270, 0), bc(11.405, 47.271, 0)}
	if got := buildProfileFromInlineZ(coords); got != nil {
		t.Errorf("want nil for all-zero Z, got %v", got)
	}
}

func TestBuildProfileFromInlineZ_TooFewPoints_ReturnsNil(t *testing.T) {
	if got := buildProfileFromInlineZ(nil); got != nil {
		t.Errorf("want nil for nil coords, got %v", got)
	}
	if got := buildProfileFromInlineZ([]brouterCoord{bc(11.4, 47.2, 580)}); got != nil {
		t.Errorf("want nil for single coord, got %v", got)
	}
}

func TestBuildProfileFromInlineZ_PartialZ_DistanceContinues(t *testing.T) {
	coords := []brouterCoord{
		bc(11.404, 47.270, 581),
		bcNoZ(11.405, 47.271),
		bc(11.406, 47.272, 600),
	}
	profile := buildProfileFromInlineZ(coords)
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points (mid-point dropped), got %d", len(profile))
	}
	if !(profile[1]["dist_m"].(float64) > profile[0]["dist_m"].(float64)) {
		t.Errorf("dist_m must keep increasing across dropped middle point")
	}
}

func TestBuildProfileFromInlineZ_SentinelZ_Filtered(t *testing.T) {
	coords := []brouterCoord{
		bc(11.404, 47.270, -9999),
		bc(11.405, 47.271, 590),
		bc(11.406, 47.272, 99999),
		bc(11.407, 47.273, 600),
	}
	profile := buildProfileFromInlineZ(coords)
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points (sentinels filtered), got %d", len(profile))
	}
	assertInDelta(t, 590.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 600.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
}

func TestBuildProfileFromInlineZ_AllSentinel_ReturnsNil(t *testing.T) {
	coords := []brouterCoord{bc(11.404, 47.270, -9999), bc(11.405, 47.271, 99999)}
	if got := buildProfileFromInlineZ(coords); got != nil {
		t.Errorf("want nil for all-sentinel Z, got %v", got)
	}
}

// ---- extractCoordinates ----

func TestExtractCoordinates_NullZIsPerCoord_NotGlobalDiscard(t *testing.T) {
	raw := `{"geometry":{"type":"LineString","coordinates":[
	  [11.404, 47.270, 581],
	  [11.405, 47.271, null],
	  [11.406, 47.272, 600]
	]}}`
	var feat map[string]any
	if err := json.Unmarshal([]byte(raw), &feat); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	coords := extractCoordinates(feat)
	if len(coords) != 3 {
		t.Fatalf("want 3 coords (null Z preserved as EleOK=false), got %d", len(coords))
	}
	if !coords[0].EleOK {
		t.Errorf("coords[0].EleOK want true")
	}
	if coords[1].EleOK {
		t.Errorf("coords[1].EleOK want false (null Z)")
	}
	if !coords[2].EleOK {
		t.Errorf("coords[2].EleOK want true")
	}
}

// ---- injectElevationProfileFromInlineZ ----

func TestInjectElevationProfileFromInlineZ_RealBrouterShape(t *testing.T) {
	raw := `{
	  "type":"FeatureCollection",
	  "features":[{
	    "type":"Feature",
	    "properties":{"creator":"BRouter-1.7.8","track-length":"118","cost":"155"},
	    "geometry":{"type":"LineString","coordinates":[[11.404,47.270,581],[11.405,47.271,590]]}
	  }]
	}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	injectElevationProfileFromInlineZ(gj, nil)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profileRaw, has := props["elevation_profile"]
	if !has {
		t.Fatal("elevation_profile key must be present")
	}
	if profileRaw == nil {
		t.Fatal("elevation_profile must NOT be nil for 3D BRouter coords")
	}
	profile := profileRaw.([]map[string]any)
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points, got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
	// Original properties must be preserved.
	if props["creator"] != "BRouter-1.7.8" {
		t.Errorf("original property 'creator' lost, got %v", props["creator"])
	}
	if props["cost"] != "155" {
		t.Errorf("original property 'cost' lost, got %v", props["cost"])
	}
}

func TestInjectElevationProfileFromInlineZ_PlainWrapperNoZ_NullProfile(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{"name":"a"},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	val, has := props["elevation_profile"]
	if !has {
		t.Fatal("elevation_profile key must be present")
	}
	if val != nil {
		t.Errorf("no Z available → elevation_profile must be nil, got %v", val)
	}
}

func TestInjectElevationProfileFromInlineZ_AlternativesMultiFeature(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":{"name":"a"},"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,590]]}},
	  {"type":"Feature","properties":{"name":"b"},"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,595]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	for i, f := range gj["features"].([]any) {
		props := f.(map[string]any)["properties"].(map[string]any)
		profileRaw, has := props["elevation_profile"]
		if !has {
			t.Errorf("feature %d missing elevation_profile key", i)
		}
		if profileRaw == nil {
			t.Errorf("feature %d must have non-nil profile", i)
		}
	}
}

func TestInjectElevationProfileFromInlineZ_NoFeaturesKey_NoOp(t *testing.T) {
	gj := map[string]any{"type": "FeatureCollection"}
	injectElevationProfileFromInlineZ(gj, nil) // must not panic
	if _, has := gj["features"]; has {
		t.Errorf("features key must remain absent")
	}
}

func TestInjectElevationProfileFromInlineZ_FeatureWithoutProperties(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,590]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	props, ok := feat["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties must be a map after injection")
	}
	if _, has := props["elevation_profile"]; !has {
		t.Errorf("elevation_profile must be present")
	}
}

func TestInjectElevationProfileFromInlineZ_PropertiesIsNull_Defensive(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":null,"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580],[11.41,47.21,590]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	props, ok := feat["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties must become a map (was null)")
	}
	if _, has := props["elevation_profile"]; !has {
		t.Errorf("elevation_profile must be present")
	}
}

func TestInjectElevationProfileFromInlineZ_PropertiesIsArray_Defensive(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":["unexpected","array"],"geometry":{"type":"LineString","coordinates":[[11.4,47.2,580]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	propsArr, isArr := feat["properties"].([]any)
	if !isArr {
		t.Fatal("properties array must be preserved")
	}
	if len(propsArr) != 2 {
		t.Errorf("want 2 elements in preserved array, got %d", len(propsArr))
	}
}

func TestInjectElevationProfileFromInlineZ_PropertiesIsString_Defensive(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","properties":"unexpected-string","geometry":{"type":"LineString","coordinates":[[11.4,47.2,580]]}}
	]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	injectElevationProfileFromInlineZ(gj, nil)
	feat := gj["features"].([]any)[0].(map[string]any)
	if feat["properties"] != "unexpected-string" {
		t.Errorf("string properties must be preserved, got %v", feat["properties"])
	}
}

func TestInjectElevationProfileFromInlineZ_NonFeatureCollection_NoOp(t *testing.T) {
	gj := map[string]any{"error": "no route possible"}
	injectElevationProfileFromInlineZ(gj, nil)
	if gj["error"] != "no route possible" {
		t.Errorf("error key must remain unchanged, got %v", gj["error"])
	}
}

// TestExtractCoordinates_MalformedCoords_PreservesDistanceConsistency verifies
// Critic finding #11 (MAJOR): when BRouter emits a null or non-array element
// inside geometry.coordinates, the distance axis in the injected elevation_profile
// is internally consistent with the wire coordinate list received by the frontend.
//
// Architecture note: extractCoordinates drops malformed coord entries (null /
// non-array / non-numeric lon-lat) because they have no valid position and
// cannot participate in Haversine accumulation. The frontend receives the original
// geometry.coordinates unchanged — including the null entry. Both sides therefore
// skip the same unusable point, so profile dist_m values match what the frontend
// would compute over the non-null wire coords. No drift.
//
// This test confirms three invariants:
//  1. geometry.coordinates in the wire GeoJSON is NOT mutated (still contains
//     the null entry at index 1).
//  2. extractCoordinates returns only the 2 valid coords (null dropped).
//  3. Profile dist_m[1] equals the Haversine distance between the 2 valid coords
//     (i.e. the distance axis is consistent with the reduced coord list that
//     both the proxy and any null-aware frontend consumer would use).
func TestExtractCoordinates_MalformedCoords_PreservesDistanceConsistency(t *testing.T) {
	// BRouter edge-case: null element between two valid 3D coords.
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature",
	  "properties":{},
	  "geometry":{"type":"LineString","coordinates":[
	    [11.404, 47.270, 581],
	    null,
	    [11.406, 47.272, 600]
	  ]}
	}]}`

	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Invariant 1: geometry.coordinates is unchanged after inject.
	injectElevationProfileFromInlineZ(gj, nil)

	feat := gj["features"].([]any)[0].(map[string]any)
	geom := feat["geometry"].(map[string]any)
	wireCoordsRaw := geom["coordinates"].([]any)
	if len(wireCoordsRaw) != 3 {
		t.Fatalf("wire geometry.coordinates: want 3 elements (incl. null), got %d", len(wireCoordsRaw))
	}
	if wireCoordsRaw[1] != nil {
		t.Errorf("wire coordinates[1] must remain nil (null in JSON), got %v", wireCoordsRaw[1])
	}

	// Invariant 2: extractCoordinates drops the null, returns 2 valid coords.
	coords := extractCoordinates(feat)
	if len(coords) != 2 {
		t.Fatalf("extractCoordinates: want 2 valid coords (null dropped), got %d", len(coords))
	}

	// Invariant 3: profile dist_m[1] = Haversine(coord0, coord1) — i.e. the
	// distance axis uses only the coords that extractCoordinates produced.
	props := feat["properties"].(map[string]any)
	profileRaw := props["elevation_profile"]
	if profileRaw == nil {
		t.Fatal("elevation_profile must not be nil (both valid coords have usable Z)")
	}
	profile := profileRaw.([]map[string]any)
	if len(profile) != 2 {
		t.Fatalf("profile: want 2 points, got %d", len(profile))
	}
	assertInDelta(t, 0.0, profile[0]["dist_m"].(float64), 0.01, "profile[0].dist_m")

	// Expected distance: Haversine between the two valid extracted coords.
	wantDist := haversineM(coords[0].Lat, coords[0].Lon, coords[1].Lat, coords[1].Lon)
	assertInDelta(t, wantDist, profile[1]["dist_m"].(float64), 1.0,
		"profile[1].dist_m must equal Haversine(coord0, coord1) — no drift")
}

// ---- DEM fallback (fillMissingElevFromDEM / injectElevationProfileFromInlineZ with DEMSvc) ----

// stubBrouterDEM implements dem.Servicer for BRouter proxy tests. It returns
// a per-position slice of decimeter values supplied at construction time.
// A position entry of dem.NoData signals absent coverage.
type stubBrouterDEM struct {
	// values is indexed by call order (i.e. the order of missing coords passed
	// in a single ElevationBatch call). Panic on out-of-bounds is intentional —
	// it catches bugs in how many coords are requested.
	values []int32
	err    error
}

func (s *stubBrouterDEM) ElevationBatch(lats, lons []int32) ([]int32, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.values[:len(lats)], nil
}

func (s *stubBrouterDEM) Health() dem.Health { return dem.Health{} }
func (s *stubBrouterDEM) Close()             {}

// TestBRouterProxy_MissingInlineZ_FallbackToService verifies that when BRouter
// sends a 2D-only response (no inline-Z), dem.Service fills the full profile.
func TestBRouterProxy_MissingInlineZ_FallbackToService(t *testing.T) {
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{"name":"test"},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271],[11.406,47.272]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// DEM returns 5810, 5900, 6000 decimeters → 581.0, 590.0, 600.0 m
	svc := &stubBrouterDEM{values: []int32{5810, 5900, 6000}}
	injectElevationProfileFromInlineZ(gj, svc)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profileRaw := props["elevation_profile"]
	if profileRaw == nil {
		t.Fatal("elevation_profile must not be nil after DEM fallback")
	}
	profile := profileRaw.([]map[string]any)
	if len(profile) != 3 {
		t.Fatalf("want 3 profile points, got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
	assertInDelta(t, 600.0, profile[2]["ele_m"].(float64), 0.01, "profile[2].ele_m")
}

// TestBRouterProxy_PartialInlineZ_OnlyMissingFilled verifies that only coords
// lacking plausible inline-Z are replaced by DEM; coords with valid Z are kept.
func TestBRouterProxy_PartialInlineZ_OnlyMissingFilled(t *testing.T) {
	// coord[1] has Z=0 (implausible per isPlausibleSrtmEle) → DEM fills it.
	// coord[0] and coord[2] have valid inline-Z and must not be changed.
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270,581],[11.405,47.271,0],[11.406,47.272,600]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// DEM is consulted only for the single missing coord[1] → 5900 dm = 590.0 m.
	svc := &stubBrouterDEM{values: []int32{5900}}
	injectElevationProfileFromInlineZ(gj, svc)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profile := props["elevation_profile"].([]map[string]any)
	if len(profile) != 3 {
		t.Fatalf("want 3 profile points (gap filled), got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m — inline-Z unchanged")
	assertInDelta(t, 590.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m — from DEM")
	assertInDelta(t, 600.0, profile[2]["ele_m"].(float64), 0.01, "profile[2].ele_m — inline-Z unchanged")
}

// TestBRouterProxy_NilDEMSvc_FallbackSkipped verifies that nil DEMSvc causes
// no crash and leaves the profile as built from inline-Z (or nil if no Z).
func TestBRouterProxy_NilDEMSvc_FallbackSkipped(t *testing.T) {
	// 2D-only BRouter response → no inline-Z.
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Must not panic and profile must stay nil (no inline-Z, no DEM service).
	injectElevationProfileFromInlineZ(gj, nil)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	val := props["elevation_profile"]
	if val != nil {
		t.Errorf("nil DEMSvc → elevation_profile must remain nil, got %v", val)
	}
}

// TestBRouterProxy_DEMNoData_NullEleM verifies that DEM NoData responses
// cause the corresponding coord to be absent from the profile (EleOK=false
// → buildProfileFromInlineZ omits the point).
func TestBRouterProxy_DEMNoData_NullEleM(t *testing.T) {
	// All three coords lack inline-Z; DEM returns NoData for coord[1].
	raw := `{"type":"FeatureCollection","features":[{
	  "type":"Feature","properties":{},
	  "geometry":{"type":"LineString","coordinates":[[11.404,47.270],[11.405,47.271],[11.406,47.272]]}
	}]}`
	var gj map[string]any
	if err := json.Unmarshal([]byte(raw), &gj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// coord[1] → NoData; coord[0] and coord[2] get real values.
	svc := &stubBrouterDEM{values: []int32{5810, dem.NoData, 6000}}
	injectElevationProfileFromInlineZ(gj, svc)

	feat := gj["features"].([]any)[0].(map[string]any)
	props := feat["properties"].(map[string]any)
	profileRaw := props["elevation_profile"]
	if profileRaw == nil {
		t.Fatal("elevation_profile must not be nil (two coords have DEM data)")
	}
	profile := profileRaw.([]map[string]any)
	// coord[1] is omitted due to NoData — only coord[0] and coord[2] remain.
	if len(profile) != 2 {
		t.Fatalf("want 2 profile points (NoData coord omitted), got %d", len(profile))
	}
	assertInDelta(t, 581.0, profile[0]["ele_m"].(float64), 0.01, "profile[0].ele_m")
	assertInDelta(t, 600.0, profile[1]["ele_m"].(float64), 0.01, "profile[1].ele_m")
}

// haversineM computes the Haversine distance in metres between two lat/lon points.
// Mirrors the geo.DistanceHaversine formula used in buildProfileFromInlineZ so
// we can assert the expected value without importing the geo package.
func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
