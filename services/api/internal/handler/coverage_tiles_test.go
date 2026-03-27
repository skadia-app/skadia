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
	ids := coverageTileIDs([4]float64{47.2, 8.3, 47.8, 8.7})
	if len(ids) != 1 {
		t.Errorf("expected 1 coverage tile, got %d", len(ids))
	}

	ids2 := coverageTileIDs([4]float64{46.9, 7.9, 48.1, 9.1})
	if len(ids2) != 4 {
		t.Errorf("expected 4 coverage tiles for boundary-spanning bbox, got %d", len(ids2))
	}
}
