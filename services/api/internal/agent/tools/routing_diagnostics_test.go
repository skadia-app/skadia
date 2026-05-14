package tools

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

func TestAnalyzeRoutePointsShortRouteComputesDistancesAndCoverageRequirements(t *testing.T) {
	result, err := executeAnalyzeRoutePoints(context.Background(), nil, "user-1", json.RawMessage(`{
		"points": [{"lat": 48.0, "lng": 11.0}, {"lat": 48.1, "lng": 11.1}],
		"intent": "distance_only"
	}`))
	if err != nil {
		t.Fatalf("executeAnalyzeRoutePoints: %v", err)
	}
	got := result.(map[string]any)
	if got["long_route_status"] != "ok" {
		t.Fatalf("long_route_status = %#v, want ok: %#v", got["long_route_status"], got)
	}
	if total := got["total_straight_line_m"].(float64); total < 13000 || total > 14000 {
		t.Fatalf("total_straight_line_m = %.1f, want about 13.4km", total)
	}
	legs := got["legs"].([]map[string]any)
	if len(legs) != 1 {
		t.Fatalf("legs = %#v, want one leg", legs)
	}
	brouter := legs[0]["brouter"].(map[string]any)
	if tiles := brouter["required_rd5_tiles"].([]string); len(tiles) == 0 {
		t.Fatalf("required rd5 tiles empty: %#v", brouter)
	}
	dem := legs[0]["dem"].(map[string]any)
	if cells := dem["required_hgt_cells"].([]string); len(cells) == 0 {
		t.Fatalf("required DEM cells empty: %#v", dem)
	}
}

func TestLongRouteRequiresWaypointsAndIgnoresModelThresholdOverride(t *testing.T) {
	result, err := executeAnalyzeRoutePoints(context.Background(), nil, "user-1", json.RawMessage(`{
		"points": [{"lat": 48.0, "lng": 11.0}, {"lat": 50.0, "lng": 13.0}],
		"intent": "direct_route",
		"max_direct_leg_m": 10000000
	}`))
	if err != nil {
		t.Fatalf("executeAnalyzeRoutePoints: %v", err)
	}
	got := result.(map[string]any)
	if got["long_route_status"] != "requires_waypoints" {
		t.Fatalf("long_route_status = %#v, want requires_waypoints: %#v", got["long_route_status"], got)
	}
	if got["max_direct_leg_m"] != float64(defaultMaxDirectRouteLegM) {
		t.Fatalf("max_direct_leg_m = %#v, want default %.0f", got["max_direct_leg_m"], float64(defaultMaxDirectRouteLegM))
	}
	if count := got["suggested_intermediate_point_count"].(int); count < 2 {
		t.Fatalf("suggested_intermediate_point_count = %d, want at least 2", count)
	}
	if math.Round(got["max_leg_m"].(float64)/1000) < 260 {
		t.Fatalf("max_leg_m = %#v, want long synthetic route", got["max_leg_m"])
	}
}

func TestLongRouteWithEnoughViaPointsIsAccepted(t *testing.T) {
	result, err := executeAnalyzeRoutePoints(context.Background(), nil, "user-1", json.RawMessage(`{
		"points": [
			{"lat": 48.0, "lng": 11.0},
			{"lat": 48.5, "lng": 11.5},
			{"lat": 49.0, "lng": 12.0},
			{"lat": 49.5, "lng": 12.5},
			{"lat": 50.0, "lng": 13.0}
		],
		"intent": "direct_route"
	}`))
	if err != nil {
		t.Fatalf("executeAnalyzeRoutePoints: %v", err)
	}
	got := result.(map[string]any)
	if got["long_route_status"] != "ok" {
		t.Fatalf("long_route_status = %#v, want ok for split legs: %#v", got["long_route_status"], got)
	}
	if got["suggested_intermediate_point_count"].(int) != 0 {
		t.Fatalf("suggested_intermediate_point_count = %#v, want 0", got["suggested_intermediate_point_count"])
	}
}
