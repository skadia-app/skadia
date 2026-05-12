package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegisterRouteToolsOwnershipSplit(t *testing.T) {
	reg := NewRegistry()
	RegisterRouteTools(reg)

	tests := map[string]bool{
		"search_routes":        true,
		"plan_route":           false,
		"search_pois_on_route": true,
		"get_isochrone":        false,
		"query_coverage":       false,
	}
	for name, wantUserOwned := range tests {
		tool := reg.tools[name]
		if tool == nil {
			t.Fatalf("tool %q was not registered", name)
		}
		if tool.Permission != PermissionRead {
			t.Fatalf("%s permission = %q, want read", name, tool.Permission)
		}
		if tool.IsUserOwned != wantUserOwned {
			t.Fatalf("%s IsUserOwned = %v, want %v", name, tool.IsUserOwned, wantUserOwned)
		}
		if err := validateSchema(name, tool.InputSchema); err != nil {
			t.Fatalf("%s schema invalid: %v", name, err)
		}
	}
}

func TestPlanRouteUsesSharedHTTPClientWithThirtySecondTimeout(t *testing.T) {
	if routeHTTPClient == nil {
		t.Fatal("routeHTTPClient is nil")
	}
	if routeHTTPClient.Timeout != 30*time.Second {
		t.Fatalf("routeHTTPClient.Timeout = %s, want 30s", routeHTTPClient.Timeout)
	}
}

func TestExecutePlanRouteHTTP(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": []any{}})
	}))
	defer server.Close()

	result, err := executePlanRoute(context.Background(), &Deps{BRouterURL: server.URL}, "", json.RawMessage(`{
		"waypoints": [[11.394,47.259],[11.450,47.280]],
		"profile": "hiking-mountain",
		"profile_params": {"SAC_scale_limit": "2", "unsafe param": "ignored"}
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	if gotPath != "/brouter" {
		t.Fatalf("path = %q, want /brouter", gotPath)
	}
	for _, want := range []string{
		"lonlats=11.394000%2C47.259000%7C11.450000%2C47.280000",
		"profile=hiking-mountain",
		"format=geojson",
		"profile%3ASAC_scale_limit=2",
	} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query missing %q in %q", want, gotQuery)
		}
	}
	if strings.Contains(gotQuery, "unsafe") {
		t.Fatalf("unsafe profile param leaked into query: %q", gotQuery)
	}
	if result.(map[string]any)["type"] != "FeatureCollection" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecuteQueryCoverageHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/coverage/query" {
			t.Fatalf("path = %q, want /internal/coverage/query", r.URL.Path)
		}
		if r.URL.Query().Get("lat") != "47.2" || r.URL.Query().Get("lon") != "11.4" || r.URL.Query().Get("layers") != "forest,traffic" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"forest": 7, "traffic": 2})
	}))
	defer server.Close()

	result, err := executeQueryCoverage(context.Background(), &Deps{MLBaseURL: server.URL}, "", json.RawMessage(`{
		"lat": 47.2,
		"lon": 11.4,
		"layers": "forest,traffic"
	}`))
	if err != nil {
		t.Fatalf("executeQueryCoverage returned error: %v", err)
	}
	got := result.(map[string]any)
	if got["lat"] != 47.2 || got["lon"] != 11.4 {
		t.Fatalf("unexpected coordinates: %#v", got)
	}
	layers := got["layers"].(map[string]any)
	if layers["forest"].(float64) != 7 || layers["traffic"].(float64) != 2 {
		t.Fatalf("unexpected layers: %#v", layers)
	}
}

func TestBuildSearchRoutesSQLAppliesUserAndActivityFilters(t *testing.T) {
	commute := true
	limitArg := 200
	args := searchRoutesArgs{LocationName: "Innsbruck", SportType: "cycling", IsCommute: &commute, Limit: &limitArg}
	sql, params, limit, err := buildSearchRoutesSQL("00000000-0000-0000-0000-000000000001", args, fixedAgentToolNow())
	if err != nil {
		t.Fatalf("buildSearchRoutesSQL returned error: %v", err)
	}
	for _, want := range []string{
		"FROM activities a",
		"a.user_id = $1",
		"(a.start_city ILIKE $3 OR a.start_state ILIKE $3)",
		"a.sport_type = $4",
		"a.is_commute = $5",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL missing %q:\n%s", want, sql)
		}
	}
	if limit != 50 {
		t.Fatalf("limit = %d, want clamp to 50", limit)
	}
	if len(params) != 5 {
		t.Fatalf("params len = %d, want 5", len(params))
	}
}

func TestBuildSearchPOIsOnRouteSQLIsRouteScopedAndUserOwnedByMetadata(t *testing.T) {
	buffer := 500
	limitArg := 200
	sql, params, limit, err := buildSearchPOIsOnRouteSQL(searchPOIsOnRouteArgs{
		RouteGeometry: `{"type":"LineString","coordinates":[[11.4,47.2],[11.5,47.3]]}`,
		BufferM:       &buffer,
		Category:      "shelter",
		Limit:         &limitArg,
	})
	if err != nil {
		t.Fatalf("buildSearchPOIsOnRouteSQL returned error: %v", err)
	}
	if strings.Contains(sql, "user_id") {
		t.Fatalf("route geometry POI SQL should not add a user_id filter:\n%s", sql)
	}
	for _, want := range []string{"FROM osm_pois", "ST_GeomFromGeoJSON($1)", "category = $3"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL missing %q:\n%s", want, sql)
		}
	}
	if limit != 50 {
		t.Fatalf("limit = %d, want clamp to 50", limit)
	}
	if len(params) != 4 {
		t.Fatalf("params len = %d, want 4", len(params))
	}
}

func TestBuildIsochroneQueriesUsePgRoutingOnly(t *testing.T) {
	nearestSQL, distanceSQL, hullSQL := buildIsochroneSQL()
	for name, sql := range map[string]string{"nearest": nearestSQL, "distance": distanceSQL, "hull": hullSQL} {
		if strings.Contains(strings.ToLower(sql), "http") || strings.Contains(strings.ToLower(sql), "coverage") {
			t.Fatalf("%s SQL must be pure DB work, got:\n%s", name, sql)
		}
	}
	if !strings.Contains(distanceSQL, "pgr_drivingDistance") {
		t.Fatalf("distance SQL must use pgr_drivingDistance:\n%s", distanceSQL)
	}
}
