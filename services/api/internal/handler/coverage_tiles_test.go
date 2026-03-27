package handler

import (
	"math"
	"testing"
)

func TestSlippyTileBBox(t *testing.T) {
	bbox := slippyTileBBox(10, 535, 362)
	if bbox[0] >= bbox[2] {
		t.Errorf("minLat(%f) should be < maxLat(%f)", bbox[0], bbox[2])
	}
	if bbox[1] >= bbox[3] {
		t.Errorf("minLon(%f) should be < maxLon(%f)", bbox[1], bbox[3])
	}
	if bbox[0] < 40 || bbox[2] > 55 {
		t.Errorf("lat range %f-%f seems wrong for z10 tile in Europe", bbox[0], bbox[2])
	}
}

func TestSlippyTileBBoxZ0(t *testing.T) {
	bbox := slippyTileBBox(0, 0, 0)
	if math.Abs(bbox[1]-(-180)) > 0.001 {
		t.Errorf("z0 minLon = %f, want -180", bbox[1])
	}
	if math.Abs(bbox[3]-180) > 0.001 {
		t.Errorf("z0 maxLon = %f, want 180", bbox[3])
	}
}

func TestCoverageTileIDs(t *testing.T) {
	// bbox within a single 1°×1° tile
	ids := coverageTileIDs([4]float64{47.2, 8.3, 47.8, 8.7})
	if len(ids) != 1 {
		t.Errorf("expected 1 coverage tile, got %d", len(ids))
	}

	// bbox spanning 2×2 → includes all 4 corner tiles (floor gives 46,47 × 7,8)
	// Plus the maxLat/maxLon floors: floor(48.1)=48, floor(9.1)=9
	// So 3 lat rows (46,47,48) × 3 lon cols (7,8,9) = 9 tiles
	ids2 := coverageTileIDs([4]float64{46.9, 7.9, 48.1, 9.1})
	if len(ids2) != 9 {
		t.Errorf("expected 9 coverage tiles for boundary-spanning bbox, got %d", len(ids2))
	}

	// bbox exactly on degree boundary — maxLat=48.0 → floor=48 → includes tile 48
	ids3 := coverageTileIDs([4]float64{47.5, 8.5, 48.0, 9.0})
	if len(ids3) != 4 {
		t.Errorf("expected 4 coverage tiles for exact-boundary bbox, got %d", len(ids3))
	}
}
