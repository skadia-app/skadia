package main

import (
	"path/filepath"
	"testing"
)

// TestExampleManifestParses guards the documented `ground_truth.yaml` schema:
// when a future change renames or removes a field on `GroundTruthCase`,
// `testdata/example_ground_truth.yaml` must be updated in the same commit.
// Without this gate the example silently rots and stops being a usable
// schema reference for the recipe in geo-lift-detection.md.
func TestExampleManifestParses(t *testing.T) {
	gt, err := loadManifest(filepath.Join("testdata", "example_ground_truth.yaml"))
	if err != nil {
		t.Fatalf("loadManifest: %v", err)
	}
	if len(gt.Cases) == 0 {
		t.Fatal("example manifest must hold at least one case to be useful")
	}
	for i, c := range gt.Cases {
		if c.File == "" {
			t.Errorf("case[%d]: file empty", i)
		}
		if c.Sport == "" {
			t.Errorf("case[%d]: sport empty", i)
		}
		if c.ExpectedRidesMin > c.ExpectedRidesMax {
			t.Errorf("case[%d]: min %d > max %d", i, c.ExpectedRidesMin, c.ExpectedRidesMax)
		}
	}
}

// TestExampleStationMarkersParse guards the documented `_gt_lifts.yaml`
// schema in the same way for the station-marker file consumed by
// `-boundaries` and `lift_detect_viz`. Each non-empty case must carry well-
// formed `[lat, lng]` pairs; an empty `lifts:` list is the conventional
// NEGATIV marker and is intentionally tolerated.
func TestExampleStationMarkersParse(t *testing.T) {
	cases := []string{
		"alpha_resort/run_a.fit",
		"beta_resort/run_b.gpx",
		"negativ/road_climb.fit",
	}
	for _, fileKey := range cases {
		c, err := loadBoundCase(filepath.Join("testdata", "example_gt_lifts.yaml"), fileKey)
		if err != nil {
			// loadBoundCase rejects empty `lifts:` lists. That is correct for
			// `-boundaries` but a perfectly legal NEGATIV entry. Tolerate it
			// here because the parser still proves the schema round-trips.
			if fileKey == "negativ/road_climb.fit" {
				continue
			}
			t.Errorf("%s: %v", fileKey, err)
			continue
		}
		for j, L := range c.Lifts {
			if L.ID == "" {
				t.Errorf("%s lift[%d]: id empty", fileKey, j)
			}
			if len(L.P1) != 2 || len(L.P2) != 2 {
				t.Errorf("%s lift[%d]: p1/p2 must be [lat, lng]", fileKey, j)
			}
		}
	}
}
