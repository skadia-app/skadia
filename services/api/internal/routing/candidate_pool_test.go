package routing

import (
	"testing"
)

// makeNode is a test helper that constructs a ReachableNode with a single
// Pareto state at the given distance.
func makeNode(id uint64, lat, lon float64, distanceM float32) ReachableNode {
	return ReachableNode{
		NodeID: id,
		Lat:    lat,
		Lon:    lon,
		States: []NodePareto{
			{DistanceM: distanceM, Cost: 1.0},
		},
	}
}

// Centre of test fixtures: Obertraubling area (48.99°N, 12.10°E).
const (
	startLat = 48.99
	startLon = 12.10
)

// TestBuildCandidatePool_EmptyReachable: empty input → nil.
func TestBuildCandidatePool_EmptyReachable(t *testing.T) {
	result := BuildCandidatePool(nil, startLat, startLon, 10000, 4, 300)
	if result != nil {
		t.Errorf("expected nil for empty reachable, got %v", result)
	}

	result = BuildCandidatePool([]ReachableNode{}, startLat, startLon, 10000, 4, 300)
	if result != nil {
		t.Errorf("expected nil for empty reachable slice, got %v", result)
	}
}

// TestBuildCandidatePool_ZeroPoolSize: poolSize <= 0 → nil.
func TestBuildCandidatePool_ZeroPoolSize(t *testing.T) {
	nodes := []ReachableNode{
		makeNode(1, 49.00, 12.11, 5000),
		makeNode(2, 48.98, 12.09, 5200),
	}
	result := BuildCandidatePool(nodes, startLat, startLon, 10000, 0, 300)
	if result != nil {
		t.Errorf("expected nil for poolSize=0, got %v", result)
	}
}

// TestBuildCandidatePool_SingleNode: single reachable node → exactly 1
// selection regardless of poolSize.
func TestBuildCandidatePool_SingleNode(t *testing.T) {
	nodes := []ReachableNode{
		makeNode(42, 49.01, 12.12, 5000),
	}
	result := BuildCandidatePool(nodes, startLat, startLon, 10000, 4, 300)
	if len(result) != 1 {
		t.Fatalf("expected 1 selection for single node, got %d", len(result))
	}
	if result[0].NodeIdx != 0 {
		t.Errorf("expected NodeIdx=0, got %d", result[0].NodeIdx)
	}
	if result[0].StateIdx != 0 {
		t.Errorf("expected StateIdx=0, got %d", result[0].StateIdx)
	}
}

// TestBuildCandidatePool_Deterministic: identical inputs produce identical
// outputs across two independent calls.
func TestBuildCandidatePool_Deterministic(t *testing.T) {
	// Four nodes spread in cardinal directions around startLat/startLon.
	nodes := []ReachableNode{
		makeNode(1, 49.04, 12.10, 4900), // north
		makeNode(2, 48.94, 12.10, 5100), // south
		makeNode(3, 48.99, 12.20, 5000), // east
		makeNode(4, 48.99, 12.00, 4800), // west
	}

	r1 := BuildCandidatePool(nodes, startLat, startLon, 10000, 4, 300)
	r2 := BuildCandidatePool(nodes, startLat, startLon, 10000, 4, 300)

	if len(r1) != len(r2) {
		t.Fatalf("non-deterministic length: first=%d second=%d", len(r1), len(r2))
	}
	for i := range r1 {
		if r1[i] != r2[i] {
			t.Errorf("slot %d differs: first=%+v second=%+v", i, r1[i], r2[i])
		}
	}
}

// TestBuildCandidatePool_MinDistConstraint: three nodes where two are very
// close together; minDist should prevent the third from being chosen, yielding
// only 2 selections.
func TestBuildCandidatePool_MinDistConstraint(t *testing.T) {
	// node 1: north  (~4.4 km from start)
	// node 2: south  (~4.4 km from start)
	// node 3: ~20 m north of node 1 — should be excluded by minDist=300
	nodes := []ReachableNode{
		makeNode(1, 49.03, 12.10, 4400), // north
		makeNode(2, 48.95, 12.10, 4500), // south
		makeNode(3, 49.030180, 12.10, 4420), // ~20 m north of node 1
	}

	// minDist = 500 m: node 3 is ~20 m from node 1 → should be rejected.
	result := BuildCandidatePool(nodes, startLat, startLon, 9000, 3, 500)

	if len(result) != 2 {
		t.Fatalf("expected 2 selections (minDist rejects node3), got %d: %v", len(result), result)
	}
	// Verify node 3 (index 2) is not in the result.
	for _, sel := range result {
		if sel.NodeIdx == 2 {
			t.Errorf("node index 2 (too close) must not appear in pool, but it did: %v", result)
		}
	}
}

// TestBuildCandidatePool_TiebreakerNodeID: two candidate nodes at symmetric
// bearings relative to the already-selected seed have identical angular spread;
// the one with the lower NodeID must be chosen.
func TestBuildCandidatePool_TiebreakerNodeID(t *testing.T) {
	// Layout (start = 48.99°N 12.10°E):
	//
	//   index 0 — seed: due north, DistanceM=5000 (exact half of 10 000 target).
	//                   NodeID=100. Chosen as seed because its distance delta is 0.
	//
	//   index 1 — nodeA (SE ≈ 135°): NodeID=200, DistanceM=6000.
	//   index 2 — nodeB (SW ≈ 225°): NodeID=50,  DistanceM=6000.
	//
	// Both candidates have the same bearing-distance from the seed's bearing
	// (0°):  |135-0|=135  and  |360-225|=135  → tie in minAngle.
	// Lower NodeID (50) must win → slot 1 should be index 2.
	nodes := []ReachableNode{
		// index 0: seed — due north
		{NodeID: 100, Lat: 49.045, Lon: 12.10, States: []NodePareto{{DistanceM: 5000, Cost: 1}}},
		// index 1: nodeA (SE ~135°) — NodeID 200
		{NodeID: 200, Lat: 48.9564, Lon: 12.1636, States: []NodePareto{{DistanceM: 6000, Cost: 1}}},
		// index 2: nodeB (SW ~225°) — NodeID 50  ← lower, should win tiebreak
		{NodeID: 50, Lat: 48.9564, Lon: 12.0364, States: []NodePareto{{DistanceM: 6000, Cost: 1}}},
	}

	// poolSize = 2: seed + 1 more; minDist=0 to avoid spatial filtering.
	result := BuildCandidatePool(nodes, startLat, startLon, 10000, 2, 0)

	if len(result) != 2 {
		t.Fatalf("expected 2 selections, got %d: %v", len(result), result)
	}
	// Slot 0 must be seed (NodeID 100, index 0).
	if result[0].NodeIdx != 0 {
		t.Errorf("expected seed (NodeID=100, idx=0) at slot 0, got NodeIdx=%d", result[0].NodeIdx)
	}
	// Slot 1 must be nodeB (NodeID 50, index 2) because 50 < 200.
	if result[1].NodeIdx != 2 {
		t.Errorf("expected nodeB (NodeID=50, idx=2) at slot 1 due to tiebreaker, got NodeIdx=%d (NodeID=%d)",
			result[1].NodeIdx, nodes[result[1].NodeIdx].NodeID)
	}
}
