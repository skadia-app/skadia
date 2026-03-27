package routing

import (
	"context"
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestLandmarkDijkstraUnified_SmallGraph(t *testing.T) {
	// 3-node linear graph: 0 --100cm,1.5x--> 1 --200cm,1.0x--> 2
	// Cost 0→1 = 100/100 * 1.5 = 1.5
	// Cost 1→2 = 200/100 * 1.0 = 2.0
	// Cost 0→2 = 1.5 + 2.0 = 3.5
	csr := &graph.UnifiedCSR{
		NodeCount: 3,
		EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2, 2},
		Edges: []graph.PrecomputedEdge{
			{Target: 1, Dist: 100, CostFactor: 1.5},
			{Target: 2, Dist: 200, CostFactor: 1.0},
		},
		NodeTileID:  []uint32{0, 0, 0},
		NodeLocalID: []uint32{0, 1, 2},
	}

	costs := landmarkDijkstraUnified(context.Background(), csr, 0, &CostConfig{})

	if costs[0] != 0 {
		t.Errorf("node 0: got %f, want 0", costs[0])
	}
	if costs[1] != 1.5 {
		t.Errorf("node 1: got %f, want 1.5", costs[1])
	}
	if costs[2] != 3.5 {
		t.Errorf("node 2: got %f, want 3.5", costs[2])
	}
}

func TestLandmarkDijkstraUnified_BlockedEdge(t *testing.T) {
	// 2-node graph with blocked edge (CostFactor >= costBlocked)
	csr := &graph.UnifiedCSR{
		NodeCount: 2,
		EdgeCount: 1,
		Offsets:   []uint32{0, 1, 1},
		Edges: []graph.PrecomputedEdge{
			{Target: 1, Dist: 100, CostFactor: costBlocked},
		},
		NodeTileID:  []uint32{0, 0},
		NodeLocalID: []uint32{0, 1},
	}

	costs := landmarkDijkstraUnified(context.Background(), csr, 0, &CostConfig{})

	if costs[0] != 0 {
		t.Errorf("node 0: got %f, want 0", costs[0])
	}
	if costs[1] != math.MaxFloat32 {
		t.Errorf("node 1: got %f, want MaxFloat32 (unreachable)", costs[1])
	}
}

func TestLandmarkDijkstraUnified_ShortestPath(t *testing.T) {
	// Diamond graph: 0→1 (cost 3), 0→2 (cost 1), 2→1 (cost 1) — shortest 0→1 = 2 via node 2
	csr := &graph.UnifiedCSR{
		NodeCount: 3,
		EdgeCount: 3,
		Offsets:   []uint32{0, 2, 2, 3},
		Edges: []graph.PrecomputedEdge{
			{Target: 1, Dist: 300, CostFactor: 1.0}, // 0→1: cost 3
			{Target: 2, Dist: 100, CostFactor: 1.0}, // 0→2: cost 1
			{Target: 1, Dist: 100, CostFactor: 1.0}, // 2→1: cost 1
		},
		NodeTileID:  []uint32{0, 0, 0},
		NodeLocalID: []uint32{0, 1, 2},
	}

	costs := landmarkDijkstraUnified(context.Background(), csr, 0, &CostConfig{})

	if costs[0] != 0 {
		t.Errorf("node 0: got %f, want 0", costs[0])
	}
	// Shortest path 0→1 is via 2: cost = 1 + 1 = 2
	if costs[1] != 2.0 {
		t.Errorf("node 1: got %f, want 2.0 (via node 2)", costs[1])
	}
	if costs[2] != 1.0 {
		t.Errorf("node 2: got %f, want 1.0", costs[2])
	}
}

func TestLandmarkDijkstraUnified_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	csr := &graph.UnifiedCSR{
		NodeCount: 2,
		EdgeCount: 1,
		Offsets:   []uint32{0, 1, 1},
		Edges: []graph.PrecomputedEdge{
			{Target: 1, Dist: 100, CostFactor: 1.0},
		},
		NodeTileID:  []uint32{0, 0},
		NodeLocalID: []uint32{0, 1},
	}

	// Should still complete for small graphs (context check is every 5M expansions)
	costs := landmarkDijkstraUnified(ctx, csr, 0, &CostConfig{})
	if costs[0] != 0 {
		t.Errorf("node 0: got %f, want 0", costs[0])
	}
}
