package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
	if geocoding["external_status"] != "unconfigured" {
		t.Fatalf("geocoding = %#v, want unconfigured external status", geocoding)
	}
	weather := got["weather"].(map[string]any)
	if weather["live_status"] != "unconfigured" {
		t.Fatalf("weather = %#v, want unconfigured live status", weather)
	}
}
