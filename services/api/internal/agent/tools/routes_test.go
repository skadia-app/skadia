package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/agent"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRegisterRouteToolsOwnershipSplit(t *testing.T) {
	reg := NewRegistry()
	RegisterRouteTools(reg)

	tests := map[string]bool{
		"search_routes":         true,
		"list_routing_profiles": false,
		"plan_route":            false,
		"search_pois_on_route":  false,
		"get_isochrone":         false,
		"query_coverage":        false,
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

func TestRoutingDomainToolsAllRegistered(t *testing.T) {
	reg := NewRegistry()
	RegisterAll(reg)

	for _, name := range agent.DomainTools["routing"] {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("routing domain tool %q is not registered", name)
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
		"profile_params": {"SAC_scale_limit": "2"}
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
	if result.(map[string]any)["type"] != "FeatureCollection" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecutePlanRouteAddsWayTagMetadata(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "FeatureCollection",
			"features": []any{map[string]any{
				"type": "Feature",
				"properties": map[string]any{
					"messages": []any{
						[]any{"Longitude", "Latitude", "Elevation", "Distance", "WayTags"},
						[]any{"11576298", "48137118", "523", "14", "highway=footway surface=paving_stones sac_scale=mountain_hiking"},
						[]any{"11576413", "48137263", "523", "19", "highway=path surface=ground mtb:scale=2 mtb:scale:uphill=1 trail_visibility=good"},
					},
				},
				"geometry": map[string]any{
					"type": "LineString",
					"coordinates": []any{
						[]any{11.576124, 48.137154, 523},
						[]any{11.576298, 48.137118, 523},
						[]any{11.576413, 48.137263, 523},
					},
				},
			}},
		})
	}))
	defer server.Close()

	result, err := executePlanRoute(context.Background(), &Deps{BRouterURL: server.URL}, "", json.RawMessage(`{
		"waypoints": [[11.576124,48.137154],[11.576413,48.137263]],
		"profile": "hiking-mountain"
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	if !strings.Contains(gotQuery, "profile%3AprocessUnusedTags=1") {
		t.Fatalf("query missing processUnusedTags=1 in %q", gotQuery)
	}

	got := result.(map[string]any)
	props := got["features"].([]any)[0].(map[string]any)["properties"].(map[string]any)
	segments, ok := props["segments"].([]map[string]any)
	if !ok {
		t.Fatalf("segments type = %T, want []map[string]any", props["segments"])
	}
	if len(segments) != 2 {
		t.Fatalf("segments len = %d, want 2", len(segments))
	}
	if segments[0]["highway"] != "footway" || segments[0]["surface"] != "paving_stones" || segments[0]["sac_scale"] != "T2" {
		t.Fatalf("first segment tags = %#v", segments[0])
	}
	if segments[1]["highway"] != "path" || segments[1]["surface"] != "ground" || segments[1]["mtb_scale"] != "S2" || segments[1]["trail_visibility"] != "good" {
		t.Fatalf("second segment tags = %#v", segments[1])
	}
	if _, ok := props["sac_ranges"]; !ok {
		t.Fatalf("sac_ranges missing from %#v", props)
	}
	if _, ok := props["mtb_ranges"]; !ok {
		t.Fatalf("mtb_ranges missing from %#v", props)
	}
}

func TestExecutePlanRouteRejectsUnknownProfileBeforeHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	result, err := executePlanRoute(context.Background(), &Deps{BRouterURL: server.URL}, "", json.RawMessage(`{
		"waypoints": [[11.394,47.259],[11.450,47.280]],
		"profile": "not-a-real-profile"
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	if called {
		t.Fatal("unknown profile should be rejected before calling BRouter")
	}
	got := result.(map[string]any)
	if got["error"] == "" || got["fallback"] != true {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestExecutePlanRouteRejectsUnknownProfileParamBeforeHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	result, err := executePlanRoute(context.Background(), &Deps{BRouterURL: server.URL}, "", json.RawMessage(`{
		"waypoints": [[11.394,47.259],[11.450,47.280]],
		"profile": "hiking-mountain",
		"profile_params": {"unknown_param": "1"}
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	if called {
		t.Fatal("unknown profile param should be rejected before calling BRouter")
	}
	got := result.(map[string]any)
	if got["error"] == "" || got["fallback"] != true {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestExecutePlanRouteRejectsInvalidWaypointBeforeHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := executePlanRoute(context.Background(), &Deps{BRouterURL: server.URL}, "", json.RawMessage(`{
		"waypoints": [[11.394,47.259],[181,47.280]],
		"profile": "hiking-mountain"
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	if called {
		t.Fatal("invalid waypoint should be rejected before calling BRouter")
	}
	got := result.(map[string]any)
	if got["error"] == "" || got["fallback"] != true {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestBuildPlanRouteURLRejectsTooManyWaypoints(t *testing.T) {
	waypoints := make([][]float64, maxPlanRouteWaypoints+1)
	for i := range waypoints {
		waypoints[i] = []float64{11.0 + float64(i)/1000, 47.0}
	}
	if _, err := buildPlanRouteURL("http://example.test", planRouteArgs{Waypoints: waypoints, Profile: "hiking-mountain"}); err == nil {
		t.Fatal("buildPlanRouteURL accepted too many waypoints")
	}
}

func TestExecutePlanRouteCapsResponseSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"payload":"` + strings.Repeat("x", maxPlanRouteResponseBytes) + `"}`))
	}))
	defer server.Close()

	result, err := executePlanRoute(context.Background(), &Deps{BRouterURL: server.URL}, "", json.RawMessage(`{
		"waypoints": [[11.394,47.259],[11.450,47.280]],
		"profile": "hiking-mountain"
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	got := result.(map[string]any)
	if got["fallback"] != true || !strings.Contains(got["error"].(string), "response too large") {
		t.Fatalf("unexpected capped response result: %#v", got)
	}
}

func TestExecutePlanRoutePreviewDoesNotUseSaveMethod(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": []any{}})
	}))
	defer server.Close()

	_, err := executePlanRoute(context.Background(), &Deps{BRouterURL: server.URL}, "", json.RawMessage(`{
		"waypoints": [[11.394,47.259],[11.450,47.280]],
		"profile": "hiking-mountain"
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET preview/no-save request", gotMethod)
	}
}

func TestExecutePlanRouteMergesRoutingProfileDefaults(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	userID := insertRouteToolUser(t, pool, "defaults")
	profileName := "agent-defaults-" + uuid.NewString()
	profile, err := db.CreateRoutingProfile(context.Background(), userID, profileName, "hiking", []sport.Predicate{{Sport: "hiking"}}, sport.ModifierFilters{}, []byte(`{
		"profile_params": {
			"consider_elevation": true,
			"SAC_scale_limit": "2"
		}
	}`))
	if err != nil {
		t.Fatalf("CreateRoutingProfile: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM routing_profiles WHERE id = $1`, profile.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "FeatureCollection", "features": []any{}})
	}))
	defer server.Close()

	result, err := executePlanRoute(context.Background(), &Deps{DB: db, BRouterURL: server.URL}, userID, json.RawMessage(`{
		"waypoints": [[11.394,47.259],[11.450,47.280]],
		"profile": "`+profile.ID+`",
		"profile_params": {"SAC_scale_limit": "3"}
	}`))
	if err != nil {
		t.Fatalf("executePlanRoute returned error: %v", err)
	}
	got := result.(map[string]any)
	if got["profile_key"] != profile.ID || got["routing_profile"] != profileName {
		t.Fatalf("unexpected routing profile metadata: %#v", got)
	}
	for _, want := range []string{
		"profile=hiking-mountain",
		"profile%3Aconsider_elevation=true",
		"profile%3ASAC_scale_limit=3",
	} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query missing %q in %q", want, gotQuery)
		}
	}
}

func TestExecuteListRoutingProfilesVisibleToUser(t *testing.T) {
	pool := testdb.Open(t)
	db := &database.DB{Pool: pool}
	userID := insertRouteToolUser(t, pool, "owner")
	foreignUserID := insertRouteToolUser(t, pool, "foreign")
	ownName := "agent-own-" + uuid.NewString()
	foreignName := "agent-foreign-" + uuid.NewString()
	globalName := "agent-global-" + uuid.NewString()
	preds := []sport.Predicate{{Sport: "hiking"}}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM routing_profiles WHERE name = ANY($1)`, []string{ownName, foreignName, globalName})
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []string{userID, foreignUserID})
	})

	if _, err := db.CreateRoutingProfile(context.Background(), userID, ownName, "hiking", preds, sport.ModifierFilters{}, []byte(`{"profile":"hiking","name":"Owner Hiking"}`)); err != nil {
		t.Fatalf("CreateRoutingProfile owner: %v", err)
	}
	if _, err := db.CreateRoutingProfile(context.Background(), foreignUserID, foreignName, "hiking", preds, sport.ModifierFilters{}, []byte(`{"profile":"hiking","name":"Foreign Hiking"}`)); err != nil {
		t.Fatalf("CreateRoutingProfile foreign: %v", err)
	}
	if _, err := db.UpsertGlobalProfile(context.Background(), globalName, "hiking", preds, sport.ModifierFilters{}, []byte(`{"profile":"hiking","name":"Global Hiking"}`), "system"); err != nil {
		t.Fatalf("UpsertGlobalProfile: %v", err)
	}

	result, err := executeListRoutingProfiles(context.Background(), &Deps{DB: db}, userID, nil)
	if err != nil {
		t.Fatalf("executeListRoutingProfiles: %v", err)
	}
	profiles := result.(map[string]any)["profiles"].([]routingProfileToolItem)
	if !hasRoutingProfileToolName(profiles, ownName) {
		t.Fatalf("own profile %q missing from %#v", ownName, profiles)
	}
	if !hasRoutingProfileToolName(profiles, globalName) {
		t.Fatalf("global profile %q missing from %#v", globalName, profiles)
	}
	if hasRoutingProfileToolName(profiles, foreignName) {
		t.Fatalf("foreign profile %q leaked into %#v", foreignName, profiles)
	}
	for _, profile := range profiles {
		if profile.Supports == nil || !profile.Supports["direct_route_preview"] {
			t.Fatalf("profile %q missing direct route support metadata: %#v", profile.Label, profile)
		}
		if len(profile.SemanticPreferences) == 0 || len(profile.ElevationSemantics) == 0 {
			t.Fatalf("profile %q missing semantic/elevation metadata: %#v", profile.Label, profile)
		}
		if profile.ProfileVersion == "" {
			t.Fatalf("profile %q missing profile version/fingerprint metadata: %#v", profile.Label, profile)
		}
	}
}

func TestExecuteQueryCoverageHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/coverage/query" {
			t.Fatalf("path = %q, want /internal/coverage/query", r.URL.Path)
		}
		if r.Header.Get("X-ML-Callback-Secret") != "secret" {
			t.Fatalf("missing internal auth header")
		}
		if r.URL.Query().Get("lat") != "47.2" || r.URL.Query().Get("lon") != "11.4" || r.URL.Query().Get("layers") != "forest,traffic" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"forest": 7, "traffic": 2})
	}))
	defer server.Close()

	oldBase := routeAPIBaseURL
	routeAPIBaseURL = server.URL
	t.Cleanup(func() { routeAPIBaseURL = oldBase })

	result, err := executeQueryCoverage(context.Background(), &Deps{MLBaseURL: "http://127.0.0.1:1", InternalSecret: "secret"}, "", json.RawMessage(`{
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

func TestExecuteQueryCoverageRejectsInvalidInputBeforeHTTP(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	oldBase := routeAPIBaseURL
	routeAPIBaseURL = server.URL
	t.Cleanup(func() { routeAPIBaseURL = oldBase })

	if _, err := executeQueryCoverage(context.Background(), nil, "", json.RawMessage(`{
		"lat": 91,
		"lon": 11.4,
		"layers": "forest"
	}`)); err == nil {
		t.Fatal("executeQueryCoverage accepted out-of-range coordinates")
	}
	if _, err := executeQueryCoverage(context.Background(), nil, "", json.RawMessage(`{
		"lat": 47.2,
		"lon": 11.4,
		"layers": "../secret"
	}`)); err == nil {
		t.Fatal("executeQueryCoverage accepted invalid layer")
	}
	if called {
		t.Fatal("invalid coverage input should be rejected before HTTP")
	}
}

func TestExecuteQueryCoverageCapsResponseSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"forest":"` + strings.Repeat("x", maxCoverageResponseBytes) + `"}`))
	}))
	defer server.Close()
	oldBase := routeAPIBaseURL
	routeAPIBaseURL = server.URL
	t.Cleanup(func() { routeAPIBaseURL = oldBase })

	_, err := executeQueryCoverage(context.Background(), nil, "", json.RawMessage(`{
		"lat": 47.2,
		"lon": 11.4,
		"layers": "forest"
	}`))
	if err == nil || !strings.Contains(err.Error(), "response too large") {
		t.Fatalf("executeQueryCoverage err = %v, want response cap error", err)
	}
}

func TestGetIsochroneSchemaDoesNotAcceptProfile(t *testing.T) {
	reg := NewRegistry()
	RegisterRouteTools(reg)
	tool := reg.tools["get_isochrone"]
	if tool == nil {
		t.Fatal("get_isochrone not registered")
	}
	if err := validateArgs("get_isochrone", tool.InputSchema, json.RawMessage(`{"lat":47.2,"lng":11.4,"profile":"mtb"}`)); err == nil {
		t.Fatal("get_isochrone schema accepted ignored profile")
	}
}

func TestBuildSearchPOIsOnRouteRejectsInvalidAndOversizedGeometry(t *testing.T) {
	if _, _, _, err := buildSearchPOIsOnRouteSQL(searchPOIsOnRouteArgs{
		RouteGeometry: `{"type":"Point","coordinates":[11.4,47.2]}`,
	}); err == nil {
		t.Fatal("buildSearchPOIsOnRouteSQL accepted non-line geometry")
	}
	if _, _, _, err := buildSearchPOIsOnRouteSQL(searchPOIsOnRouteArgs{
		RouteGeometry: `{"type":"LineString","coordinates":[[11.4,47.2],[181,47.3]]}`,
	}); err == nil {
		t.Fatal("buildSearchPOIsOnRouteSQL accepted out-of-range coordinate")
	}
	coords := make([]string, maxRouteGeometryVertices+1)
	for i := range coords {
		coords[i] = `[11.4,47.2]`
	}
	if _, _, _, err := buildSearchPOIsOnRouteSQL(searchPOIsOnRouteArgs{
		RouteGeometry: `{"type":"LineString","coordinates":[` + strings.Join(coords, ",") + `]}`,
	}); err == nil {
		t.Fatal("buildSearchPOIsOnRouteSQL accepted too many vertices")
	}
}

func TestBuildIsochroneQueryCapsReachablePoints(t *testing.T) {
	_, distanceSQL, _ := buildIsochroneSQL()
	if !strings.Contains(distanceSQL, "LIMIT 1000") {
		t.Fatalf("distance SQL missing point cap:\n%s", distanceSQL)
	}
}

func hasRoutingProfileToolName(profiles []routingProfileToolItem, name string) bool {
	for _, p := range profiles {
		if p.Label == name || p.Key == name {
			return true
		}
	}
	return false
}

func insertRouteToolUser(t *testing.T, pool *pgxpool.Pool, prefix string) string {
	t.Helper()
	userID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, prefix+"-"+userID[:8], prefix+"-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
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
