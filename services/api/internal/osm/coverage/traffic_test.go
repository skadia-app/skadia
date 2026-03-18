package coverage

import (
	"testing"

	"github.com/paulmach/orb"
)

func TestClassifyTraffic(t *testing.T) {
	tests := []struct {
		raw  float64
		want uint8
	}{
		{0, 0}, {3, 1}, {8, 2}, {15, 3}, {30, 4}, {50, 5}, {100, 6},
	}
	for _, tc := range tests {
		got := classifyTraffic(tc.raw)
		if got != tc.want {
			t.Errorf("classifyTraffic(%v) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

func TestTrafficDivisionFormula(t *testing.T) {
	// High population, low motorway density → high traffic
	raw := trafficRaw(100, 10, 5, 20)
	if raw < 10 {
		t.Errorf("high pop + low motorway should be high traffic, got %f", raw)
	}

	// Same population, high motorway density → lower traffic (motorway absorbs)
	rawWithMW := trafficRaw(100, 10, 50, 20)
	if rawWithMW >= raw {
		t.Errorf("motorway should reduce traffic: %f >= %f", rawWithMW, raw)
	}
}

func TestFloatGridBasics(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := newFloatGrid(bbox, 100.0)

	if g.Rows <= 0 || g.Cols <= 0 {
		t.Fatalf("grid should have positive dimensions: %dx%d", g.Rows, g.Cols)
	}

	// Sample empty grid
	v := g.sample(orb.Point{8.5, 47.5})
	if v != 0 {
		t.Errorf("empty grid should sample 0, got %f", v)
	}

	// Set a cell and sample
	row, col := g.pointToCell(orb.Point{8.5, 47.5})
	if row < 0 {
		t.Fatal("point should be in grid")
	}
	g.Data[row*g.Cols+col] = 42.5
	v = g.sample(orb.Point{8.5, 47.5})
	if v != 42.5 {
		t.Errorf("should sample 42.5, got %f", v)
	}
}

func TestPopulateDecayFloatAdditive(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := newFloatGrid(bbox, 100.0)

	// Two cities at same location — should ADD (not max like urbanization)
	applyPopulateDecayFloat(g, orb.Point{8.5, 47.5}, 50000)
	v1 := g.sample(orb.Point{8.5, 47.5})
	applyPopulateDecayFloat(g, orb.Point{8.5, 47.5}, 50000)
	v2 := g.sample(orb.Point{8.5, 47.5})

	if v2 <= v1 {
		t.Errorf("additive: second city should increase value: v1=%f v2=%f", v1, v2)
	}
	// Should roughly double
	if v2 < v1*1.8 || v2 > v1*2.2 {
		t.Errorf("additive: should roughly double: v1=%f v2=%f ratio=%f", v1, v2, v2/v1)
	}
}

func TestIndustrialDecayFloat(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := newFloatGrid(bbox, 100.0)

	applyIndustrialDecayFloat(g, orb.Point{8.5, 47.5}, 100000)

	center := g.sample(orb.Point{8.5, 47.5})
	if center == 0 {
		t.Error("center should be non-zero for 100k m² industrial area")
	}

	far := g.sample(orb.Point{8.5, 47.6})
	if far >= center {
		t.Errorf("far (%f) should be < center (%f)", far, center)
	}
}

func TestTrafficClassification(t *testing.T) {
	// Verify boundary values
	if classifyTraffic(-1) != 0 {
		t.Error("negative should be class 0")
	}
	if classifyTraffic(5.9) != 1 {
		t.Error("5.9 should be class 1")
	}
	if classifyTraffic(6.0) != 2 {
		t.Error("6.0 should be class 2")
	}
	if classifyTraffic(69.9) != 5 {
		t.Error("69.9 should be class 5")
	}
	if classifyTraffic(70.0) != 6 {
		t.Error("70.0 should be class 6")
	}
}
