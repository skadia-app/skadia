package coverage

import (
	"testing"

	"github.com/paulmach/orb"
)

func TestUrbanizationDecay(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	g := NewGrid(bbox, 100.0)
	ApplyUrbanizationDecay(g, orb.Point{8.5, 47.5}, 100000)
	center := g.SamplePoint(orb.Point{8.5, 47.5})
	// BRouter formula with max() aggregation: single city of 100k ≈ 73 at center
	if center < 50 {
		t.Errorf("center should be >50 for 100k city, got %d", center)
	}
	if center == 0 {
		t.Error("center should not be zero")
	}
	// Verify decay: edge should be less than center
	edge := g.SamplePoint(orb.Point{8.6, 47.6})
	if edge >= center {
		t.Errorf("edge (%d) should be < center (%d)", edge, center)
	}
}

func TestUrbanizationMaxNotAdditive(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}

	// Grid with one city
	g1 := NewGrid(bbox, 100.0)
	ApplyUrbanizationDecay(g1, orb.Point{8.5, 47.5}, 100000)
	single := g1.SamplePoint(orb.Point{8.5, 47.5})

	// Grid with two nearby cities of same size — max() should give same value at center
	g2 := NewGrid(bbox, 100.0)
	ApplyUrbanizationDecay(g2, orb.Point{8.5, 47.5}, 100000)
	ApplyUrbanizationDecay(g2, orb.Point{8.501, 47.501}, 100000) // ~100m away
	double := g2.SamplePoint(orb.Point{8.5, 47.5})

	// With max() aggregation, adding a second identical city shouldn't increase the center value
	// (it was additive before, which caused saturation)
	if double > single+5 { // allow small tolerance from the slightly offset second city
		t.Errorf("max() aggregation: two nearby cities (%d) should ≈ one city (%d), not additive", double, single)
	}
}

func TestForestOverrideReducesNotZeroes(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{9.0, 48.0}}
	urban := NewGrid(bbox, 100.0)
	forest := NewGrid(bbox, 100.0)
	r, c := forest.PointToCell(orb.Point{8.5, 47.5})
	forest.Set(r, c, 1)
	ApplyUrbanizationDecay(urban, orb.Point{8.5, 47.5}, 100000)
	before := urban.SamplePoint(orb.Point{8.5, 47.5})
	ApplyForestWaterOverride(urban, forest, nil)
	after := urban.SamplePoint(orb.Point{8.5, 47.5})
	if after == 0 {
		t.Error("forest override should reduce, not zero")
	}
	if after > before/2 {
		t.Errorf("after (%d) should be much less than before (%d)", after, before)
	}
}

func TestNoiseDecayQuadraticCombination(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{8.01, 47.01}}
	g := NewGrid(bbox, 25.0)

	source := orb.Point{8.001, 47.001}
	probe := orb.Point{8.004, 47.004}

	ApplyNoiseDecay(g, source, 500.0, 15.0)
	v1 := g.SamplePoint(probe)
	if v1 == 0 || v1 == 255 {
		t.Fatalf("first source at probe should be non-zero non-saturated, got %d", v1)
	}

	ApplyNoiseDecay(g, source, 500.0, 25.0)
	v2 := g.SamplePoint(probe)
	if v2 <= v1 {
		t.Errorf("combined (%d) should be > single source (%d)", v2, v1)
	}
}

func TestNoiseDecayLine(t *testing.T) {
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{8.05, 47.05}}
	g := NewGrid(bbox, 50.0)

	// Horizontal road through the middle
	road := orb.LineString{{8.01, 47.025}, {8.04, 47.025}}
	ApplyNoiseDecayLine(g, road, 300.0, 15.0)

	// Point on the road should have noise
	onRoad := g.SamplePoint(orb.Point{8.025, 47.025})
	if onRoad == 0 {
		t.Error("point on road should have noise")
	}

	// Point 200m away should have less noise
	away := g.SamplePoint(orb.Point{8.025, 47.027}) // ~220m north
	if away >= onRoad {
		t.Errorf("point away (%d) should be < on road (%d)", away, onRoad)
	}

	// Point far away should have no noise
	far := g.SamplePoint(orb.Point{8.025, 47.045}) // ~2.2km north
	if far != 0 {
		t.Errorf("point far away should be 0, got %d", far)
	}
}

func TestEquirectDist(t *testing.T) {
	p1 := orb.Point{8.0, 47.0}
	p2 := orb.Point{8.01, 47.01}
	cosLat := 0.6819984 // cos(47°)

	approx := equirectDist(p1, p2, cosLat)
	// Should be ~1.2km (diagonal of ~0.01° at 47°N)
	if approx < 1000 || approx > 1500 {
		t.Errorf("equirectDist should be ~1.2km, got %.0fm", approx)
	}
}

func TestPopulationFallback(t *testing.T) {
	if PopulationFallback("city") != 100000 {
		t.Error("city fallback should be 100000")
	}
	if PopulationFallback("village") != 500 {
		t.Error("village fallback should be 500")
	}
	if PopulationFallback("unknown_type") != 0 {
		t.Error("unknown should be 0")
	}
}
