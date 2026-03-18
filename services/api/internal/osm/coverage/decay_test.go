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
	// BRouter formula: 10000 * 100000 * cos(47.5°) / (8*sqrt(100000) + 500 + 0)² ≈ 73
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
	// Use a small bbox with fine resolution so buffer covers cells
	bbox := orb.Bound{Min: orb.Point{8.0, 47.0}, Max: orb.Point{8.01, 47.01}}
	g := NewGrid(bbox, 25.0)

	// Place source at one corner, sample ~300m away where values don't saturate
	source := orb.Point{8.001, 47.001}
	probe := orb.Point{8.004, 47.004} // ~400m away

	ApplyNoiseDecay(g, source, 500.0, 15.0)
	v1 := g.SamplePoint(probe)
	if v1 == 0 || v1 == 255 {
		t.Fatalf("first source at probe should be non-zero non-saturated, got %d", v1)
	}

	// Apply second source at same location
	ApplyNoiseDecay(g, source, 500.0, 25.0)
	v2 := g.SamplePoint(probe)
	if v2 <= v1 {
		t.Errorf("combined (%d) should be > single source (%d)", v2, v1)
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
