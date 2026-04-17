package routing

import (
	"math"
	"testing"
)

// makeDeterministicFixture builds 10 ReachableNode values arranged in a circle
// at roughly 5 km from (48, 11). Each node has exactly one Pareto state with
// varying metrics so the semantic scorers can produce meaningful rankings.
func makeDeterministicFixture() []ReachableNode {
	nodes := make([]ReachableNode, 10)
	for i := range nodes {
		angle := float64(i) * 36.0 * math.Pi / 180.0
		nodes[i] = ReachableNode{
			NodeID: uint64(i + 1),
			Lat:    48.0 + 0.045*math.Cos(angle),
			Lon:    11.0 + 0.06*math.Sin(angle),
			States: []NodePareto{
				{
					DistanceM:    4000 + float32(i)*200,
					AvgFlat:      1.0 + float32(i)*0.1,
					ForestPct:    float32(i) * 0.1,
					NoiseScore:   0.5,
					TrafficClass: 0.3,
				},
			},
		}
	}
	return nodes
}

// TestSelectVariants_Deterministic verifies that SelectVariants produces
// identical output when called twice with the same inputs.
func TestSelectVariants_Deterministic(t *testing.T) {
	reachable := makeDeterministicFixture()
	ctx := VariantSelectionContext{TargetDistanceM: 10000}
	s1 := SelectVariants(reachable, 48.0, 11.0, DefaultVariants, 3, ctx)
	s2 := SelectVariants(reachable, 48.0, 11.0, DefaultVariants, 3, ctx)
	if len(s1) != len(s2) {
		t.Fatalf("different lengths: %d vs %d", len(s1), len(s2))
	}
	for i := range s1 {
		if s1[i].Node.NodeID != s2[i].Node.NodeID {
			t.Errorf("non-deterministic at %d: %d vs %d", i, s1[i].Node.NodeID, s2[i].Node.NodeID)
		}
	}
}

// TestSelectVariants_RespectsMaxVariants verifies the output never exceeds maxVariants.
func TestSelectVariants_RespectsMaxVariants(t *testing.T) {
	reachable := makeDeterministicFixture()
	ctx := VariantSelectionContext{TargetDistanceM: 10000}
	for _, max := range []int{1, 2, 3, 5} {
		got := SelectVariants(reachable, 48.0, 11.0, DefaultVariants, max, ctx)
		if len(got) > max {
			t.Errorf("maxVariants=%d: got %d results", max, len(got))
		}
	}
}

// TestSelectVariants_SpatialDiversity verifies that no two selected nodes are
// within MinVariantDistanceM of each other.
func TestSelectVariants_SpatialDiversity(t *testing.T) {
	reachable := makeDeterministicFixture()
	ctx := VariantSelectionContext{TargetDistanceM: 10000}
	selected := SelectVariants(reachable, 48.0, 11.0, DefaultVariants, 5, ctx)
	minDist := MinVariantDistanceM(ctx.TargetDistanceM)

	for i := range selected {
		for j := i + 1; j < len(selected); j++ {
			a := selected[i].Node
			b := selected[j].Node
			// Approximate check: lat/lon differences, 1 degree ≈ 111 km
			dlat := math.Abs(a.Lat-b.Lat) * 111000
			dlon := math.Abs(a.Lon-b.Lon) * 74000 // ~cos(48°)*111000
			dist := math.Sqrt(dlat*dlat + dlon*dlon)
			if dist < minDist*0.9 { // 10% tolerance for the rough approximation
				t.Errorf("nodes %d and %d are only ~%.0f m apart (minDist=%.0f)",
					a.NodeID, b.NodeID, dist, minDist)
			}
		}
	}
}

// TestSelectVariants_EmptyReachable handles the degenerate case of no nodes.
func TestSelectVariants_EmptyReachable(t *testing.T) {
	ctx := VariantSelectionContext{TargetDistanceM: 10000}
	got := SelectVariants(nil, 48.0, 11.0, DefaultVariants, 3, ctx)
	if len(got) != 0 {
		t.Errorf("expected 0 results for empty reachable, got %d", len(got))
	}
}

// TestSelectVariants_ZeroTargetDistance checks that scorers that require a
// positive TargetDistanceM (shortest, balanced) gracefully skip and the
// fallback fills via geometric bearing spread.
func TestSelectVariants_ZeroTargetDistance(t *testing.T) {
	reachable := makeDeterministicFixture()
	ctx := VariantSelectionContext{TargetDistanceM: 0}
	got := SelectVariants(reachable, 48.0, 11.0, DefaultVariants, 3, ctx)
	// Should not panic; any number of results 0..3 is acceptable.
	if len(got) > 3 {
		t.Errorf("expected at most 3 results, got %d", len(got))
	}
}

// TestMinVariantDistanceM checks boundary conditions.
func TestMinVariantDistanceM(t *testing.T) {
	// Floor: short distances must still produce ≥300 m
	if d := MinVariantDistanceM(0); d != 300.0 {
		t.Errorf("MinVariantDistanceM(0) = %.1f, want 300.0", d)
	}
	if d := MinVariantDistanceM(1000); d != 300.0 {
		t.Errorf("MinVariantDistanceM(1000) = %.1f, want 300.0 (5%% = 50 m < floor)", d)
	}
	// Above floor: 5% of 10 km = 500 m
	if d := MinVariantDistanceM(10000); d != 500.0 {
		t.Errorf("MinVariantDistanceM(10000) = %.1f, want 500.0", d)
	}
}

// TestFillWithGeometricFallback_BearingDiversity verifies that when no semantic
// variants fire, the geometric fallback fills slots by maximising bearing spread.
func TestFillWithGeometricFallback_BearingDiversity(t *testing.T) {
	// Place 4 nodes in cardinal directions at equal distance from start.
	reachable := []ReachableNode{
		{NodeID: 1, Lat: 48.045, Lon: 11.0, States: []NodePareto{{DistanceM: 5000}}}, // north
		{NodeID: 2, Lat: 47.955, Lon: 11.0, States: []NodePareto{{DistanceM: 5000}}}, // south
		{NodeID: 3, Lat: 48.0, Lon: 11.09, States: []NodePareto{{DistanceM: 5000}}},  // east
		{NodeID: 4, Lat: 48.0, Lon: 10.91, States: []NodePareto{{DistanceM: 5000}}},  // west
	}

	ctx := VariantSelectionContext{TargetDistanceM: 10000}
	// Use empty variants slice → all slots filled by geometric fallback.
	got := SelectVariants(reachable, 48.0, 11.0, []SemanticVariant{}, 3, ctx)
	if len(got) != 3 {
		t.Fatalf("expected 3 geometric fallback results, got %d", len(got))
	}
	for _, sv := range got {
		if sv.Variant.ID != "geometric" {
			t.Errorf("expected geometric variant, got %q", sv.Variant.ID)
		}
	}
}
