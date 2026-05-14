package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNaturalRoutingToolsMetadata(t *testing.T) {
	reg := NewRegistry()
	RegisterRouteTools(reg)

	for _, name := range []string{
		"resolve_route_place",
		"resolve_route_feature",
		"get_routing_coverage",
		"plan_route_between",
		"suggest_route_goal",
	} {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("tool %q was not registered", name)
		}
		if tool.Permission != PermissionRead {
			t.Fatalf("%s permission = %q, want read", name, tool.Permission)
		}
		if tool.ExecuteRead == nil {
			t.Fatalf("%s ExecuteRead is nil", name)
		}
		if err := validateSchema(name, tool.InputSchema); err != nil {
			t.Fatalf("%s schema invalid: %v", name, err)
		}
	}
}

func TestRouteResolverRefRoundTripAndRejectsForeignRefs(t *testing.T) {
	ref := routeResolverRef{Kind: "settlement", Source: "osm_pois", Name: "Synthetic Start", Lat: 48.0, Lng: 11.0}
	encoded := encodeRouteResolverRef(ref)
	decoded, err := decodeRouteResolverRef(encoded)
	if err != nil {
		t.Fatalf("decodeRouteResolverRef: %v", err)
	}
	if decoded.Kind != ref.Kind || decoded.Source != ref.Source || decoded.Name != ref.Name || decoded.Lat != ref.Lat || decoded.Lng != ref.Lng {
		t.Fatalf("decoded = %#v, want %#v", decoded, ref)
	}
	if _, err := decodeRouteResolverRef("not-a-route-ref"); err == nil {
		t.Fatal("decodeRouteResolverRef accepted a foreign ref")
	}
}

func TestBuildResolveRoutePlaceSQLUsesImportedMapDataAndNamedCoordinateOrder(t *testing.T) {
	limit := 30
	nearLat, nearLng := 48.0, 11.0
	sql, params := buildResolveRoutePlaceSQL("Synthetic", "all", &routePoint{Lat: &nearLat, Lng: &nearLng}, &routeBBox{
		MinLat: 47.9,
		MinLng: 10.9,
		MaxLat: 48.2,
		MaxLng: 11.3,
	}, limit)

	for _, want := range []string{
		"FROM osm_ways",
		"FROM osm_pois",
		"ST_SetSRID(ST_Point($2,$3),4326)",
		"ST_MakeEnvelope($4,$5,$6,$7,4326)",
		"ORDER BY distance_m NULLS LAST",
		"LIMIT $8",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL missing %q:\n%s", want, sql)
		}
	}
	if len(params) != 8 {
		t.Fatalf("params len = %d, want query + near lng/lat + bbox lng/lat + limit", len(params))
	}
	if params[1] != nearLng || params[2] != nearLat {
		t.Fatalf("near params = %#v, want lng then lat", params[1:3])
	}
	if params[3] != 10.9 || params[4] != 47.9 || params[5] != 11.3 || params[6] != 48.2 {
		t.Fatalf("bbox params = %#v, want min_lng,min_lat,max_lng,max_lat", params[3:7])
	}
	if params[7] != limit {
		t.Fatalf("limit param = %#v, want %d", params[7], limit)
	}
}

func TestBuildResolveRouteFeatureSQLFiltersSemanticFeatureKinds(t *testing.T) {
	sql, params := buildResolveRouteFeatureSQL("Synthetic Lake", "lake", nil, nil, 5)
	if !strings.Contains(sql, "category = 'still_water'") {
		t.Fatalf("feature SQL missing lake filter:\n%s", sql)
	}
	if !strings.Contains(sql, "ele AS ele_m") || !strings.Contains(sql, "geometry_kind") {
		t.Fatalf("feature SQL missing elevation or geometry metadata:\n%s", sql)
	}
	if len(params) != 2 {
		t.Fatalf("params len = %d, want query and limit", len(params))
	}
}

func TestBuildResolveRouteFeatureSQLIncludesNamedTrailRelations(t *testing.T) {
	sql, params := buildResolveRouteFeatureSQL("Synthetic Trail", "trail", nil, nil, 5)
	for _, want := range []string{
		"FROM osm_routes",
		"route AS subtype",
		"route IN ('hiking','bicycle','mtb','foot','running','via_ferrata')",
		"'trail' AS kind",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("trail feature SQL missing %q:\n%s", want, sql)
		}
	}
	if len(params) != 2 {
		t.Fatalf("params len = %d, want query and limit", len(params))
	}
}

func TestGetRoutingCoverageUsesResolverRefsAndComputesTilesAndDEMCells(t *testing.T) {
	segmentsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(segmentsDir, "E10_N45.rd5"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write rd5 fixture: %v", err)
	}
	demDir := t.TempDir()
	for _, cell := range []string{"N47E010", "N47E011", "N48E010", "N48E011"} {
		cellDir := filepath.Join(demDir, "sonny-05")
		if err := os.MkdirAll(cellDir, 0o755); err != nil {
			t.Fatalf("mkdir DEM fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(cellDir, cell+".hgt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write DEM fixture: %v", err)
		}
	}
	t.Setenv("BROUTER_SEGMENTS_DIR", segmentsDir)
	t.Setenv("DEM_DATA_DIR", demDir)

	ref := encodeRouteResolverRef(routeResolverRef{Kind: "settlement", Source: "osm_pois", Name: "Synthetic Start", Lat: 48.0, Lng: 11.0})
	result, err := executeGetRoutingCoverage(context.Background(), nil, "", json.RawMessage(`{
		"place_refs": ["`+ref+`"],
		"intent": "direct"
	}`))
	if err != nil {
		t.Fatalf("executeGetRoutingCoverage: %v", err)
	}
	got := result.(map[string]any)
	if got["can_resolve_place"] != true || got["can_plan_direct"] != true {
		t.Fatalf("unexpected coverage status: %#v", got)
	}
	brouter := got["brouter"].(map[string]any)
	tiles := brouter["required_rd5_tiles"].([]string)
	if len(tiles) != 1 || tiles[0] != "E10_N45" {
		t.Fatalf("tiles = %#v, want E10_N45", tiles)
	}
	if missing := brouter["missing_rd5_tiles"].([]string); len(missing) != 0 {
		t.Fatalf("missing rd5 tiles = %#v, want none", missing)
	}
	dem := got["dem"].(map[string]any)
	cells := dem["required_hgt_cells"].([]string)
	for _, want := range []string{"N47E010", "N47E011", "N48E010", "N48E011"} {
		if !containsRoutingTestString(cells, want) {
			t.Fatalf("DEM cells = %#v, want %s", cells, want)
		}
	}
	if dem["status"] != "complete" {
		t.Fatalf("DEM status = %#v, want complete", dem["status"])
	}
	if missing := dem["missing_hgt_cells"].([]string); len(missing) != 0 {
		t.Fatalf("missing DEM cells = %#v, want none", missing)
	}
}

func TestGetRoutingCoverageReportsMissingLocalRoutingData(t *testing.T) {
	t.Setenv("BROUTER_SEGMENTS_DIR", t.TempDir())
	t.Setenv("DEM_DATA_DIR", t.TempDir())

	result, err := executeGetRoutingCoverage(context.Background(), nil, "", json.RawMessage(`{
		"points": [{"lat": 48.0, "lng": 11.0}],
		"intent": "direct"
	}`))
	if err != nil {
		t.Fatalf("executeGetRoutingCoverage: %v", err)
	}
	got := result.(map[string]any)
	if got["can_plan_direct"] != false || got["blocking_reason"] != "missing_brouter_segments" {
		t.Fatalf("unexpected coverage status: %#v", got)
	}
	brouter := got["brouter"].(map[string]any)
	if missing := brouter["missing_rd5_tiles"].([]string); len(missing) != 1 || missing[0] != "E10_N45" {
		t.Fatalf("missing rd5 tiles = %#v, want E10_N45", missing)
	}
}

func TestGetRoutingCoverageUsesConfiguredDirsForBadRouterAndCoverageLayers(t *testing.T) {
	segmentsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(segmentsDir, "E10_N45.rd5"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write rd5 fixture: %v", err)
	}
	demDir := t.TempDir()
	demCellDir := filepath.Join(demDir, "sonny-05")
	if err := os.MkdirAll(demCellDir, 0o755); err != nil {
		t.Fatalf("mkdir DEM fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(demCellDir, "N48E011.hgt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write DEM fixture: %v", err)
	}
	routingTileDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(routingTileDir, "tile_48_11.bin"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write routing tile fixture: %v", err)
	}
	coverageDir := t.TempDir()
	for _, layer := range []string{"forest", "water"} {
		if err := os.MkdirAll(filepath.Join(coverageDir, layer), 0o755); err != nil {
			t.Fatalf("mkdir coverage layer fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(coverageDir, layer, "48_11.bin"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write coverage layer fixture: %v", err)
		}
	}

	result, err := executeGetRoutingCoverage(context.Background(), &Deps{
		BRouterSegmentsDir: segmentsDir,
		DEMDataDir:         demDir,
		RoutingTilesDir:    routingTileDir,
		CoverageBaseDir:    coverageDir,
	}, "", json.RawMessage(`{
		"bbox": {"min_lat": 48.1, "min_lng": 11.1, "max_lat": 48.2, "max_lng": 11.2},
		"intent": "goal"
	}`))
	if err != nil {
		t.Fatalf("executeGetRoutingCoverage: %v", err)
	}
	got := result.(map[string]any)
	if got["can_suggest_goal"] != false {
		t.Fatalf("can_suggest_goal = %#v, want false until agent goal suggestions are wired: %#v", got["can_suggest_goal"], got)
	}
	badrouter := got["badrouter"].(map[string]any)
	if badrouter["available"] != true {
		t.Fatalf("badrouter = %#v, want available", badrouter)
	}
	coverageLayers := got["coverage_layers"].(map[string]any)
	present := coverageLayers["present_layers"].([]string)
	for _, want := range []string{"forest", "water"} {
		if !containsRoutingTestString(present, want) {
			t.Fatalf("present coverage layers = %#v, want %s", present, want)
		}
	}
}

func TestPlanRouteBetweenConvertsNamedLatLngToLegacyLonLatPreview(t *testing.T) {
	segmentsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(segmentsDir, "E10_N45.rd5"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write rd5 fixture: %v", err)
	}
	startRef := encodeRouteResolverRef(routeResolverRef{Kind: "settlement", Source: "osm_pois", Name: "Synthetic Start", Lat: 48.0, Lng: 11.0})
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": []any{}})
	}))
	defer server.Close()

	result, err := executePlanRouteBetween(context.Background(), &Deps{BRouterURL: server.URL, BRouterSegmentsDir: segmentsDir}, "user-1", json.RawMessage(`{
		"from": {"ref": "`+startRef+`"},
		"to": {"lat": 48.1, "lng": 11.2},
		"profile_key": "hiking-mountain"
	}`))
	if err != nil {
		t.Fatalf("executePlanRouteBetween: %v", err)
	}
	for _, want := range []string{
		"lonlats=11.000000%2C48.000000%7C11.200000%2C48.100000",
		"profile=hiking-mountain",
	} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query missing %q in %q", want, gotQuery)
		}
	}
	got := result.(map[string]any)
	if got["agent_tool"] != "plan_route_between" {
		t.Fatalf("agent_tool = %#v, want plan_route_between", got["agent_tool"])
	}
}

func TestPlanRouteBetweenRejectsUnsupportedSemanticConstraintsBeforeHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	result, err := executePlanRouteBetween(context.Background(), &Deps{BRouterURL: server.URL}, "user-1", json.RawMessage(`{
		"from": {"lat": 48.0, "lng": 11.0},
		"to": {"lat": 48.1, "lng": 11.2},
		"profile_key": "hiking-mountain",
		"constraints": {"minimum_forest_share": 60}
	}`))
	if err != nil {
		t.Fatalf("executePlanRouteBetween: %v", err)
	}
	if called {
		t.Fatal("unsupported semantic constraints should be rejected before HTTP")
	}
	got := result.(map[string]any)
	if got["unsupported_constraint"] != true || got["fallback"] != true {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestSuggestRouteGoalReturnsStructuredUnsupportedState(t *testing.T) {
	result, err := executeSuggestRouteGoal(context.Background(), nil, "user-1", json.RawMessage(`{
		"start": {"lat": 48.0, "lng": 11.0},
		"round_trip": true,
		"target_ascent_m": 800
	}`))
	if err != nil {
		t.Fatalf("executeSuggestRouteGoal: %v", err)
	}
	got := result.(map[string]any)
	if got["status"] != "unsupported_backend_path" {
		t.Fatalf("status = %#v, want unsupported_backend_path", got["status"])
	}
	if got["target_ascent_m"] == nil {
		t.Fatalf("result missing target_ascent_m: %#v", got)
	}
}

func containsRoutingTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
