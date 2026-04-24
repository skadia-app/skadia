package handler

import (
	"encoding/json"
	"math"
	"testing"
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

	injectElevationProfileFromInlineZ(gj)

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
	injectElevationProfileFromInlineZ(gj)
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
	injectElevationProfileFromInlineZ(gj)
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
	injectElevationProfileFromInlineZ(gj) // must not panic
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
	injectElevationProfileFromInlineZ(gj)
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
	injectElevationProfileFromInlineZ(gj)
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
	injectElevationProfileFromInlineZ(gj)
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
	injectElevationProfileFromInlineZ(gj)
	feat := gj["features"].([]any)[0].(map[string]any)
	if feat["properties"] != "unexpected-string" {
		t.Errorf("string properties must be preserved, got %v", feat["properties"])
	}
}

func TestInjectElevationProfileFromInlineZ_NonFeatureCollection_NoOp(t *testing.T) {
	gj := map[string]any{"error": "no route possible"}
	injectElevationProfileFromInlineZ(gj)
	if gj["error"] != "no route possible" {
		t.Errorf("error key must remain unchanged, got %v", gj["error"])
	}
}
