package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/geosearch"
)

func TestGetAgentServiceStatusShapeAndLongDistancePolicy(t *testing.T) {
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

	result, err := executeGetAgentServiceStatus(context.Background(), &Deps{
		BRouterSegmentsDir: segmentsDir,
		DEMDataDir:         demDir,
		RoutingTilesDir:    t.TempDir(),
		CoverageBaseDir:    t.TempDir(),
	}, "user-1", json.RawMessage(`{
		"services": ["geocoding", "weather", "routing", "coverage"],
		"points": [{"lat": 48.1, "lng": 11.1}, {"lat": 48.2, "lng": 11.2}]
	}`))
	if err != nil {
		t.Fatalf("executeGetAgentServiceStatus: %v", err)
	}
	got := result.(map[string]any)
	for _, key := range []string{"geocoding", "weather", "routing", "coverage", "long_distance_policy", "difficulty_support", "blocking_reasons", "recommended_next_step"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("status missing %q: %#v", key, got)
		}
	}
	policy := got["long_distance_policy"].(map[string]any)
	if policy["max_direct_leg_m"] != float64(defaultMaxDirectRouteLegM) {
		t.Fatalf("max_direct_leg_m = %#v, want %.0f", policy["max_direct_leg_m"], float64(defaultMaxDirectRouteLegM))
	}
	if policy["model_override_allowed"] != false {
		t.Fatalf("model_override_allowed = %#v, want false", policy["model_override_allowed"])
	}
	geocoding := got["geocoding"].(map[string]any)
	if geocoding["external_status"] != "disabled" {
		t.Fatalf("geocoding = %#v, want disabled external status", geocoding)
	}
	weather := got["weather"].(map[string]any)
	if weather["live_status"] != "disabled" {
		t.Fatalf("weather = %#v, want disabled live status", weather)
	}
	if weather["status"] != "degraded" {
		t.Fatalf("weather status = %#v, want degraded: %#v", weather["status"], weather)
	}
	if !containsAnyString(weather["query_kinds"], "dated_place") || !containsAnyString(weather["query_kinds"], "current_place") {
		t.Fatalf("weather query_kinds = %#v, want place/date capability", weather["query_kinds"])
	}
	if weather["user_message"] == "" {
		t.Fatalf("weather user_message missing: %#v", weather)
	}
}

func TestGetAgentServiceStatusReflectsExternalGeocodeAndWeatherConfiguration(t *testing.T) {
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_POLICY", "allowed")
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_PROVIDER_ENABLED", "true")
	t.Setenv("SKADIA_AGENT_LIVE_WEATHER_LIMIT_PER_HOUR", "12")

	result, err := executeGetAgentServiceStatus(context.Background(), &Deps{
		GeoSearchExternal: &geocodePlaceForwardProvider{},
		GeoSearchPolicy:   geosearch.ExternalPolicyAllowed,
		GeoSearchQuota:    geosearch.NewLimiter(7, time.Hour, geocodePlaceFixedClock()),
		RouteGoalSuggester: func(context.Context, string, RouteGoalSuggestionRequest) (RouteGoalSuggestionResponse, error) {
			return RouteGoalSuggestionResponse{}, nil
		},
	}, "user-1", json.RawMessage(`{"services":["geocoding","weather","routing"]}`))
	if err != nil {
		t.Fatalf("executeGetAgentServiceStatus: %v", err)
	}
	got := result.(map[string]any)
	geocoding := got["geocoding"].(map[string]any)
	if geocoding["external_status"] != "configured" {
		t.Fatalf("geocoding = %#v, want configured external status", geocoding)
	}
	rateLimit := geocoding["rate_limit"].(map[string]any)
	if rateLimit["status"] != geosearch.QuotaStatusAvailable || rateLimit["limit_per_hour"] != 7 {
		t.Fatalf("geocoding rate_limit = %#v, want available limit 7", rateLimit)
	}
	weather := got["weather"].(map[string]any)
	if weather["live_status"] != "configured" {
		t.Fatalf("weather = %#v, want configured live status", weather)
	}
	if weather["status"] != "ok" {
		t.Fatalf("weather status = %#v, want ok: %#v", weather["status"], weather)
	}
	if !containsAnyString(weather["capabilities"], "dated_place") {
		t.Fatalf("weather capabilities = %#v, want dated_place", weather["capabilities"])
	}
	weatherRate := weather["rate_limit"].(map[string]any)
	if weatherRate["limit_per_hour"] != 12 {
		t.Fatalf("weather rate_limit = %#v, want env limit 12", weatherRate)
	}
	routing := got["routing"].(map[string]any)
	if routing["route_suggestion_status"] != "configured_round_trip_only" {
		t.Fatalf("routing = %#v, want configured round-trip route suggestion", routing)
	}
}

func TestGetAgentServiceStatusProbesRoutingService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result, err := executeGetAgentServiceStatus(context.Background(), &Deps{BRouterURL: server.URL}, "user-1", json.RawMessage(`{"services":["routing"]}`))
	if err != nil {
		t.Fatalf("executeGetAgentServiceStatus: %v", err)
	}
	got := result.(map[string]any)
	routing := got["routing"].(map[string]any)
	if routing["brouter_status"] != "reachable" || routing["brouter_probe_error"] != "" {
		t.Fatalf("routing = %#v, want reachable without probe error", routing)
	}
	if routing["status"] != "degraded" || !containsAnyString(routing["needs"], "route_suggestion_service") {
		t.Fatalf("routing = %#v, want degraded route suggestion need", routing)
	}
}

func TestGetAgentServiceStatusReportsUnreachableRoutingService(t *testing.T) {
	result, err := executeGetAgentServiceStatus(context.Background(), &Deps{BRouterURL: "http://127.0.0.1:1"}, "user-1", json.RawMessage(`{"services":["routing"]}`))
	if err != nil {
		t.Fatalf("executeGetAgentServiceStatus: %v", err)
	}
	got := result.(map[string]any)
	routing := got["routing"].(map[string]any)
	if routing["brouter_status"] != "unreachable" || routing["brouter_probe_error"] == "" {
		t.Fatalf("routing = %#v, want unreachable with probe error", routing)
	}
	if routing["status"] != "unavailable" || !containsAnyString(routing["needs"], "routing_service") {
		t.Fatalf("routing = %#v, want unavailable routing service need", routing)
	}
}

func TestGetAgentServiceStatusGeocodingReportsLookupOrderAndLimits(t *testing.T) {
	result, err := executeGetAgentServiceStatus(context.Background(), &Deps{}, "user-1", json.RawMessage(`{"services":["geocoding"]}`))
	if err != nil {
		t.Fatalf("executeGetAgentServiceStatus: %v", err)
	}
	got := result.(map[string]any)
	geocoding := got["geocoding"].(map[string]any)
	if geocoding["status"] != "unavailable" {
		t.Fatalf("geocoding status = %#v, want unavailable: %#v", geocoding["status"], geocoding)
	}
	if !containsAnyString(geocoding["lookup_order"], "local_map") || !containsAnyString(geocoding["lookup_order"], "external") {
		t.Fatalf("lookup_order = %#v, want local and external lookup steps", geocoding["lookup_order"])
	}
	if geocoding["next_action"] == "" {
		t.Fatalf("missing geocoding next_action: %#v", geocoding)
	}
}
