// dp_test.go
package graph

import (
	"math"
	"testing"
)

func TestDouglasPeuckerStraightLine(t *testing.T) {
	// 5 points on a straight line → only keep start and end
	lats := []int32{470000000, 470250000, 470500000, 470750000, 471000000}
	lons := []int32{80000000, 80250000, 80500000, 80750000, 81000000}
	cosLat := math.Cos(47.0 * math.Pi / 180.0)

	mask := DouglasPeucker(lats, lons, cosLat, 0.4)
	if len(mask) != 5 {
		t.Fatalf("expected mask len 5, got %d", len(mask))
	}
	if !mask[0] || !mask[4] {
		t.Error("start and end must always be kept")
	}
	// Inner points are collinear → should be removed
	for i := 1; i < 4; i++ {
		if mask[i] {
			t.Errorf("point %d should be removed (collinear)", i)
		}
	}
}

func TestDouglasPeuckerWithDeviation(t *testing.T) {
	// 3 points: start, deviated middle, end
	// Middle point ~10m off the line → kept at 0.4m threshold
	lats := []int32{470000000, 470500000, 471000000}
	lons := []int32{80000000, 80060000, 81000000} // middle shifted ~6m east
	cosLat := math.Cos(47.0 * math.Pi / 180.0)

	mask := DouglasPeucker(lats, lons, cosLat, 0.4)
	if !mask[1] {
		t.Error("middle point with ~6m deviation should be kept (threshold 0.4m)")
	}
}

func TestDouglasPeuckerTwoPoints(t *testing.T) {
	lats := []int32{470000000, 471000000}
	lons := []int32{80000000, 81000000}
	cosLat := math.Cos(47.0 * math.Pi / 180.0)

	mask := DouglasPeucker(lats, lons, cosLat, 0.4)
	if len(mask) != 2 || !mask[0] || !mask[1] {
		t.Error("2-point line: both must be kept")
	}
}

func TestDouglasPeuckerOnePoint(t *testing.T) {
	lats := []int32{470000000}
	lons := []int32{80000000}

	mask := DouglasPeucker(lats, lons, 0.7, 0.4)
	if len(mask) != 1 || !mask[0] {
		t.Error("single point must be kept")
	}
}

func TestComputeElevSegDistBasic(t *testing.T) {
	// 3 points ~111m apart (1° lat difference at equator)
	lats := []int32{0, 10000000, 20000000} // 0°, 1°, 2°
	lons := []int32{0, 0, 0}
	cosLat := 1.0 // equator

	dists := ComputeElevSegDist(lats, lons, cosLat)
	if len(dists) != 2 {
		t.Fatalf("expected 2 segment distances, got %d", len(dists))
	}
	// ~111km between 1° lat → 11100000 cm → clamped to 65535
	if dists[0] != 65535 {
		t.Errorf("expected clamped 65535, got %d", dists[0])
	}
}

func TestComputeElevSegDistShortSegments(t *testing.T) {
	// Points ~10m apart (typical for OSM ways)
	lats := []int32{470000000, 470000900, 470001800} // ~10m, ~10m
	lons := []int32{80000000, 80000000, 80000000}
	cosLat := math.Cos(47.0 * math.Pi / 180.0)

	dists := ComputeElevSegDist(lats, lons, cosLat)
	if len(dists) != 2 {
		t.Fatalf("expected 2, got %d", len(dists))
	}
	// ~10m = ~1000cm
	for i, d := range dists {
		if d < 800 || d > 1200 {
			t.Errorf("segment %d: expected ~1000cm, got %d", i, d)
		}
	}
}

func TestComputeElevSegDistSinglePoint(t *testing.T) {
	dists := ComputeElevSegDist([]int32{470000000}, []int32{80000000}, 0.7)
	if len(dists) != 0 {
		t.Errorf("single point should produce 0 distances, got %d", len(dists))
	}
}
