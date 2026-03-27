package routing

import (
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestAngleDiff(t *testing.T) {
	tests := []struct {
		a, b float64
		want float64
	}{
		{0, 0, 0},
		{0, math.Pi, math.Pi},
		{-math.Pi + 0.1, math.Pi - 0.1, 0.2},
		{math.Pi / 4, -math.Pi / 4, math.Pi / 2},
		{0, math.Pi / 2, math.Pi / 2},
	}
	for _, tt := range tests {
		got := angleDiff(tt.a, tt.b)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("angleDiff(%.2f, %.2f) = %.4f, want %.4f", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestFindFlankLandmarks_BasicNorthSouth(t *testing.T) {
	// Synthetic scenario: crossing at center (0,0) ele=2000dm (200m)
	// North valley: node at (0.01, 0) ele=500dm (50m), cost=20000
	// South valley: node at (-0.01, 0) ele=500dm (50m), cost=20000
	// East ridge: node at (0, 0.01) ele=2500dm (250m), cost=15000
	crossingSeed := graph.LandmarkCandidate{
		GlobalNodeID: graph.MakeGlobalNodeID(1, 0),
		Lat:          0,
		Lon:          0,
		Ele:          2000, // 200m in dm
	}

	tiles := []graph.TileScanResult{{
		TileID:    1,
		NodeCount: 4,
		NodeLats:  []int32{0, 1000000, -1000000, 0}, // E7: 0, +0.1°, -0.1°, 0
		NodeLons:  []int32{0, 0, 0, 1000000},        // E7: 0, 0, 0, +0.1°
		NodeEles:  []int32{2000, 500, 500, 2500},    // dm
	}}

	nodeMapping := map[uint32][]uint32{
		1: {0, 1, 2, 3}, // identity mapping
	}

	// Cost array: indexed by dense node ID (= local ID here)
	costs := []float32{0, 20000, 20000, 15000}

	flankA, flankB := FindFlankLandmarks(crossingSeed, costs, tiles, nodeMapping)

	// Expect flanks in north and south valleys (nodes 1 and 2), not on the ridge (node 3)
	hasNorth := (flankA.Lat == 1000000 || flankB.Lat == 1000000)
	hasSouth := (flankA.Lat == -1000000 || flankB.Lat == -1000000)
	if !hasNorth || !hasSouth {
		t.Errorf("expected flanks in N/S valleys, got A=%+v B=%+v", flankA, flankB)
	}
}

func TestFindFlankLandmarks_NoValleyFallback(t *testing.T) {
	// All valley-candidate nodes are on bearings within 60 deg (both NE of crossing).
	// Phase 1 finds deepest[0] but rejects deepest[1] (angleDiff < pi/3),
	// triggering the N/S fallback split (splitBearing=0).
	// Phase 3 finds at least one flank. Since all lower nodes are NE,
	// only one half-plane has candidates — this is the expected behavior
	// when the topology only has valleys on one side.
	crossingSeed := graph.LandmarkCandidate{
		GlobalNodeID: graph.MakeGlobalNodeID(1, 0),
		Lat:          0, Lon: 0, Ele: 5000, // 500m
	}
	tiles := []graph.TileScanResult{{
		TileID:    1,
		NodeCount: 3,
		NodeLats:  []int32{0, 500000, 600000},
		NodeLons:  []int32{0, 100000, 200000},
		// Both NE: bearings ~11° and ~18° → angleDiff < 60° → only 1 valley dir
		NodeEles: []int32{5000, 1000, 1200},
	}}
	nodeMapping := map[uint32][]uint32{1: {0, 1, 2}}
	costs := []float32{0, 20000, 20000}

	flankA, flankB := FindFlankLandmarks(crossingSeed, costs, tiles, nodeMapping)

	// At least one flank should be found (in the NE half-plane).
	// The other half-plane (SW) has no candidates — flankB may be zero.
	if flankA.GlobalNodeID == 0 && flankB.GlobalNodeID == 0 {
		t.Error("expected at least one flank from N/S fallback, got none")
	}
	// Verify the found flank is one of the NE nodes (not the crossing itself)
	found := flankA
	if found.GlobalNodeID == 0 {
		found = flankB
	}
	if found.Ele >= crossingSeed.Ele {
		t.Errorf("flank should be lower than crossing, got ele=%d (crossing=%d)", found.Ele, crossingSeed.Ele)
	}
}

func TestFindFlankLandmarks_CostFiltering(t *testing.T) {
	// Nodes with costs outside the 5-30km / 15-50km windows must be ignored
	crossingSeed := graph.LandmarkCandidate{
		GlobalNodeID: graph.MakeGlobalNodeID(1, 0),
		Lat:          0, Lon: 0, Ele: 5000, // 500m
	}
	tiles := []graph.TileScanResult{{
		TileID:    1,
		NodeCount: 4,
		NodeLats:  []int32{0, 1000000, -1000000, 2000000},
		NodeLons:  []int32{0, 0, 0, 0},
		NodeEles:  []int32{5000, 1000, 1000, 1000}, // valleys at 100m
	}}
	nodeMapping := map[uint32][]uint32{1: {0, 1, 2, 3}}
	// Node 1: cost 3000 (too close for phase 3, but in valley detection range)
	// Node 2: cost 20000 (in range for both phases)
	// Node 3: cost 60000 (too far for phase 3)
	costs := []float32{0, 3000, 20000, 60000}

	flankA, flankB := FindFlankLandmarks(crossingSeed, costs, tiles, nodeMapping)

	// Only node 2 (cost=20000) should be a flank candidate
	// Node 1 (cost=3000) is below the 15km phase-3 minimum
	// Node 3 (cost=60000) exceeds the 50km phase-3 maximum
	foundNode2 := (flankA.Lat == -1000000 || flankB.Lat == -1000000)
	foundNode3 := (flankA.Lat == 2000000 || flankB.Lat == 2000000)
	if !foundNode2 {
		t.Error("expected node 2 (cost=20000) as flank, not found")
	}
	if foundNode3 {
		t.Error("node 3 (cost=60000) should have been filtered out")
	}
}

func TestFindFlankLandmarks_CrossingLowerThanNeighbors(t *testing.T) {
	// Crossing lower than all neighbors (reversed topology) — no valleys found
	crossingSeed := graph.LandmarkCandidate{
		GlobalNodeID: graph.MakeGlobalNodeID(1, 0),
		Lat:          0, Lon: 0, Ele: 500, // 50m — lower than neighbors
	}
	tiles := []graph.TileScanResult{{
		TileID:    1,
		NodeCount: 3,
		NodeLats:  []int32{0, 1000000, -1000000},
		NodeLons:  []int32{0, 0, 0},
		NodeEles:  []int32{500, 2000, 2000}, // neighbors HIGHER
	}}
	nodeMapping := map[uint32][]uint32{1: {0, 1, 2}}
	costs := []float32{0, 20000, 20000}

	flankA, flankB := FindFlankLandmarks(crossingSeed, costs, tiles, nodeMapping)

	// Both flanks should be zero (no node is lower than crossing)
	if flankA.GlobalNodeID != 0 || flankB.GlobalNodeID != 0 {
		t.Errorf("expected no flanks for reversed topology, got A=%+v B=%+v", flankA, flankB)
	}
}
