package coverage

import (
	"testing"

	"github.com/paulmach/orb"
)

func TestRasterizeSquare(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{0, 0}, Max: orb.Point{1, 1}}
	g := NewGrid(bbox, 1120.0) // ~100 cells
	square := orb.Polygon{orb.Ring{
		{0.2, 0.2}, {0.8, 0.2}, {0.8, 0.8}, {0.2, 0.8}, {0.2, 0.2},
	}}
	RasterizePolygon(g, square, 1)
	center := g.SamplePoint(orb.Point{0.5, 0.5})
	if center != 1 {
		t.Errorf("center should be filled, got %d", center)
	}
	corner := g.SamplePoint(orb.Point{0.05, 0.05})
	if corner != 0 {
		t.Errorf("corner should be empty, got %d", corner)
	}
}

func TestRasterizeWithHole(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{0, 0}, Max: orb.Point{1, 1}}
	g := NewGrid(bbox, 1120.0)
	polyWithHole := orb.Polygon{
		orb.Ring{{0.1, 0.1}, {0.9, 0.1}, {0.9, 0.9}, {0.1, 0.9}, {0.1, 0.1}},
		orb.Ring{{0.4, 0.4}, {0.6, 0.4}, {0.6, 0.6}, {0.4, 0.6}, {0.4, 0.4}},
	}
	RasterizePolygon(g, polyWithHole, 1)
	outer := g.SamplePoint(orb.Point{0.3, 0.3})
	hole := g.SamplePoint(orb.Point{0.5, 0.5})
	if outer != 1 {
		t.Errorf("outer ring should be filled, got %d", outer)
	}
	if hole != 0 {
		t.Errorf("hole should be empty, got %d", hole)
	}
}

func TestRasterizePolygonMax(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{0, 0}, Max: orb.Point{1, 1}}
	g := NewGrid(bbox, 1120.0)
	p1 := orb.Polygon{orb.Ring{{0.2, 0.2}, {0.8, 0.2}, {0.8, 0.8}, {0.2, 0.8}, {0.2, 0.2}}}
	p2 := orb.Polygon{orb.Ring{{0.3, 0.3}, {0.7, 0.3}, {0.7, 0.7}, {0.3, 0.7}, {0.3, 0.3}}}
	RasterizePolygonMax(g, p1, 1)
	RasterizePolygonMax(g, p2, 3)
	val := g.SamplePoint(orb.Point{0.5, 0.5})
	if val != 3 {
		t.Errorf("expected max=3, got %d", val)
	}
	edge := g.SamplePoint(orb.Point{0.25, 0.25})
	if edge != 1 {
		t.Errorf("edge area should be 1, got %d", edge)
	}
}

func TestRasterizeTriangle(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{0, 0}, Max: orb.Point{1, 1}}
	g := NewGrid(bbox, 1120.0)
	tri := orb.Polygon{orb.Ring{
		{0.2, 0.2}, {0.8, 0.2}, {0.5, 0.8}, {0.2, 0.2},
	}}
	RasterizePolygon(g, tri, 1)
	inside := g.SamplePoint(orb.Point{0.5, 0.4})
	if inside != 1 {
		t.Errorf("inside triangle should be filled, got %d", inside)
	}
	outside := g.SamplePoint(orb.Point{0.1, 0.7})
	if outside != 0 {
		t.Errorf("outside triangle should be empty, got %d", outside)
	}
}

func TestRasterizeEmptyPolygon(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{0, 0}, Max: orb.Point{1, 1}}
	g := NewGrid(bbox, 1120.0)
	// Empty polygon — should not panic
	RasterizePolygon(g, orb.Polygon{}, 1)
	// Too-small ring
	RasterizePolygon(g, orb.Polygon{orb.Ring{{0.5, 0.5}, {0.5, 0.5}}}, 1)
}

func TestRasterizeOverlappingPolygons(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{0, 0}, Max: orb.Point{1, 1}}
	g := NewGrid(bbox, 1120.0)
	// Two overlapping squares — second should overwrite
	p1 := orb.Polygon{orb.Ring{{0.2, 0.2}, {0.6, 0.2}, {0.6, 0.6}, {0.2, 0.6}, {0.2, 0.2}}}
	p2 := orb.Polygon{orb.Ring{{0.4, 0.4}, {0.8, 0.4}, {0.8, 0.8}, {0.4, 0.8}, {0.4, 0.4}}}
	RasterizePolygon(g, p1, 1)
	RasterizePolygon(g, p2, 2)
	// Overlap region should have value 2 (last write wins)
	overlap := g.SamplePoint(orb.Point{0.5, 0.5})
	if overlap != 2 {
		t.Errorf("overlap should be 2 (last write), got %d", overlap)
	}
	// Only in p1
	onlyP1 := g.SamplePoint(orb.Point{0.3, 0.3})
	if onlyP1 != 1 {
		t.Errorf("only-p1 area should be 1, got %d", onlyP1)
	}
}

func TestAggregationMode(t *testing.T) {
	samples := []uint8{2, 3, 2, 3, 2, OutOfBounds, 1}
	m := Mode(samples)
	if m != 2 {
		t.Errorf("mode should be 2, got %d", m)
	}
}

func TestAggregationWeightedAvg(t *testing.T) {
	samples := []uint8{10, 20, 30, OutOfBounds}
	avg := WeightedAvg(samples)
	if avg != 20 {
		t.Errorf("weighted avg should be 20, got %d", avg)
	}
}

func TestAggregationMaxVal(t *testing.T) {
	samples := []uint8{5, 100, 50, OutOfBounds, 200}
	m := MaxVal(samples)
	if m != 200 {
		t.Errorf("max should be 200, got %d", m)
	}
}
