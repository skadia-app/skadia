package geo

import (
	"testing"

	"github.com/paulmach/orb"
)

func TestSimplifyDouglasPeucker(t *testing.T) {
	t.Run("collinear points below threshold are removed", func(t *testing.T) {
		// Straight line from west to east with a tiny 1m deviation in the middle.
		// All points should reduce to just start and end.
		ls := orb.LineString{
			{8.5000, 47.3000},
			{8.5100, 47.3000 + 0.000001}, // ~0.1m deviation from straight line
			{8.5200, 47.3000},
			{8.5300, 47.3000 + 0.000001},
			{8.5400, 47.3000},
		}
		result := SimplifyDouglasPeucker(ls, 10.0) // 10m threshold
		if len(result) > 2 {
			t.Errorf("expected 2 points after simplification, got %d", len(result))
		}
	})

	t.Run("significant deviation above threshold is preserved", func(t *testing.T) {
		// Triangle: start, a point 500m off the baseline, end.
		// The middle point deviates far from baseline → should be kept.
		ls := orb.LineString{
			{8.5000, 47.3000},
			{8.5100, 47.3045}, // ~500m north deviation
			{8.5200, 47.3000},
		}
		result := SimplifyDouglasPeucker(ls, 10.0) // 10m threshold, deviation >> threshold
		if len(result) < 3 {
			t.Errorf("expected 3 points preserved, got %d", len(result))
		}
	})

	t.Run("fewer than 3 points returned unchanged", func(t *testing.T) {
		ls1 := orb.LineString{}
		if r := SimplifyDouglasPeucker(ls1, 10.0); len(r) != 0 {
			t.Errorf("empty: got %d points, want 0", len(r))
		}

		ls2 := orb.LineString{{8.5, 47.3}}
		if r := SimplifyDouglasPeucker(ls2, 10.0); len(r) != 1 {
			t.Errorf("single point: got %d points, want 1", len(r))
		}

		ls3 := orb.LineString{{8.5, 47.3}, {8.6, 47.4}}
		if r := SimplifyDouglasPeucker(ls3, 10.0); len(r) != 2 {
			t.Errorf("two points: got %d points, want 2", len(r))
		}
	})
}
