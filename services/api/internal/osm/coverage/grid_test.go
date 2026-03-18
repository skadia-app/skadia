package coverage

import (
	"testing"

	"github.com/paulmach/orb"
)

func TestNewGrid(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := NewGrid(bbox, 25.0)
	if g.Rows < 4000 || g.Rows > 5000 {
		t.Errorf("expected ~4450 rows, got %d", g.Rows)
	}
	if g.Cols < 2500 || g.Cols > 4000 {
		t.Errorf("expected ~3020 cols, got %d", g.Cols)
	}
}

func TestSamplePointInBounds(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := NewGrid(bbox, 100.0)
	r, c := g.PointToCell(orb.Point{8.5, 47.5})
	g.Set(r, c, 42)
	if g.SamplePoint(orb.Point{8.5, 47.5}) != 42 {
		t.Error("center cell should be 42")
	}
}

func TestSamplePointOutOfBounds(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := NewGrid(bbox, 100.0)
	if g.SamplePoint(orb.Point{7.0, 46.0}) != OutOfBounds {
		t.Error("expected OutOfBounds sentinel")
	}
}

func TestSampleLineAmortized(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := NewGrid(bbox, 100.0)
	// Fill a horizontal band
	for c := range g.Cols {
		g.Set(g.Rows/2, c, 1)
	}
	line := orb.LineString{{8.2, 47.5}, {8.8, 47.5}}
	samples := g.SampleLine(line, 100.0)
	if len(samples) < 5 {
		t.Fatalf("expected >=5 samples, got %d", len(samples))
	}
	hits := 0
	for _, v := range samples {
		if v == 1 {
			hits++
		}
	}
	if hits == 0 {
		t.Error("expected hits in filled band")
	}
}

func TestAggregationIgnoresOutOfBounds(t *testing.T) {
	samples := []uint8{1, 0, OutOfBounds, 1, OutOfBounds}
	pct := PctNonZero(samples)
	// 3 valid, 2 non-zero → 66%
	if pct != 66 {
		t.Errorf("expected 66%%, got %d%%", pct)
	}
}
