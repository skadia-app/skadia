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
