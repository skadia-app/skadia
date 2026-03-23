package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// newBidiTestGraph builds a bidirectional linear graph: 0↔1↔2↔3↔4
//
// Node 0: edges to [1]       Offsets[0]=0, Offsets[1]=1
// Node 1: edges to [0, 2]    Offsets[1]=1, Offsets[2]=3
// Node 2: edges to [1, 3]    Offsets[2]=3, Offsets[3]=5
// Node 3: edges to [2, 4]    Offsets[3]=5, Offsets[4]=7
// Node 4: edges to [3]       Offsets[4]=7, Offsets[5]=8
// Targets: [1, 0, 2, 1, 3, 2, 4, 3]
func newBidiTestGraph() *SingleGraphReader {
	// highway=track (enum 15) is passable for all profiles.
	const trackTags1 uint64 = 15
	g := &graph.Graph{
		NodeCount: 5,
		EdgeCount: 8,
		Offsets:   []uint32{0, 1, 3, 5, 7, 8},
		Targets:   []uint32{1, 0, 2, 1, 3, 2, 4, 3},
		EdgeDist:  []uint32{1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000},
		EdgeTags1: []uint64{trackTags1, trackTags1, trackTags1, trackTags1, trackTags1, trackTags1, trackTags1, trackTags1},
		EdgeTags2: make([]uint64, 8),
		NodeLat:   []int32{470000000, 470010000, 470020000, 470030000, 470040000},
		NodeLon:   []int32{110000000, 110000000, 110000000, 110000000, 110000000},
		NodeEle:   []int32{1000, 1000, 1000, 1000, 1000},
		IsBorder:  make([]bool, 5),
		NodeTags:  make([]byte, 5),
	}
	g.BuildReverseAdj()
	return &SingleGraphReader{G: g}
}

func TestRunBackwardSearchBasic(t *testing.T) {
	reader := newBidiTestGraph()
	cfg := DefaultCostConfig()
	result := RunBackwardSearch(reader, 4, cfg, 100000)

	if cost, ok := result.LowerBound[4]; !ok || cost > 0.01 {
		t.Errorf("dst cost = %v, want ~0", cost)
	}
	if _, ok := result.LowerBound[0]; !ok {
		t.Error("node 0 should be reachable")
	}
}

func TestRunBackwardSearchRespectsUpperBound(t *testing.T) {
	reader := newBidiTestGraph()
	cfg := DefaultCostConfig()
	// Each edge is 10m (1000cm) with default costfactor ~1.0 → ~10 cost units per hop.
	// Upper bound of 5 is too small to reach node 0 (4 hops away).
	result := RunBackwardSearch(reader, 4, cfg, 5)

	if _, ok := result.LowerBound[0]; ok {
		t.Error("node 0 should not be reachable with tiny upper bound")
	}
}

func TestRunBackwardSearchDisconnected(t *testing.T) {
	// Node 2 has no outgoing edges — backward search from 2 can't expand.
	g := &graph.Graph{
		NodeCount: 3,
		EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2, 2}, // Node 2 has no outgoing edges
		Targets:   []uint32{1, 0},
		EdgeDist:  []uint32{1000, 1000},
		EdgeTags1: []uint64{15, 15}, // highway=track
		EdgeTags2: make([]uint64, 2),
		NodeLat:   []int32{470000000, 470010000, 470020000},
		NodeLon:   []int32{110000000, 110000000, 110000000},
		NodeEle:   []int32{1000, 1000, 1000},
		IsBorder:  make([]bool, 3),
		NodeTags:  make([]byte, 3),
	}
	g.BuildReverseAdj()
	reader := &SingleGraphReader{G: g}
	cfg := DefaultCostConfig()
	result := RunBackwardSearch(reader, 2, cfg, 100000)
	// Node 2 has incoming edges (0→2 is not present, only 0→1, 1→0), so backward
	// via reverse edges: node 2 has no incoming edges → only itself in LowerBound
	if len(result.LowerBound) != 1 {
		t.Errorf("expected 1 node, got %d", len(result.LowerBound))
	}
}

func TestRunBackwardSearchDirectedGraph(t *testing.T) {
	// Directed graph: 0→1→2→3 (no reverse edges)
	const trackTags1 uint64 = 15
	g := &graph.Graph{
		NodeCount: 4,
		EdgeCount: 3,
		Offsets:   []uint32{0, 1, 2, 3, 3},
		Targets:   []uint32{1, 2, 3},
		EdgeDist:  []uint32{1000, 1000, 1000},
		EdgeTags1: []uint64{trackTags1, trackTags1, trackTags1},
		EdgeTags2: make([]uint64, 3),
		NodeLat:   []int32{470000000, 470010000, 470020000, 470030000},
		NodeLon:   []int32{110000000, 110000000, 110000000, 110000000},
		NodeEle:   []int32{1000, 1000, 1000, 1000},
		IsBorder:  make([]bool, 4),
		NodeTags:  make([]byte, 4),
	}
	g.BuildReverseAdj()
	reader := &SingleGraphReader{G: g}
	cfg := DefaultCostConfig()

	// Backward from node 3: should find 2→3, 1→2, 0→1 (via reverse edges)
	result := RunBackwardSearch(reader, MakeGlobalNodeID(0, 3), cfg, 100000)

	// All nodes should be reachable via reverse traversal
	for i := range 4 {
		nodeID := MakeGlobalNodeID(0, uint32(i))
		if _, ok := result.LowerBound[nodeID]; !ok {
			t.Errorf("node %d should be reachable via reverse edges", i)
		}
	}

	// Backward from node 0: no incoming edges → only node 0 reachable
	result0 := RunBackwardSearch(reader, MakeGlobalNodeID(0, 0), cfg, 100000)
	if len(result0.LowerBound) != 1 {
		t.Errorf("backward from node 0: expected 1 reachable, got %d", len(result0.LowerBound))
	}
}

func TestForEachInEdgeDirected(t *testing.T) {
	// Directed graph: 0→1→2 (no reverse forward edges)
	const trackTags1 uint64 = 15
	g := &graph.Graph{
		NodeCount: 3,
		EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2, 2},
		Targets:   []uint32{1, 2},
		EdgeDist:  []uint32{1000, 2000},
		EdgeTags1: []uint64{trackTags1, trackTags1},
		EdgeTags2: make([]uint64, 2),
		NodeLat:   []int32{470000000, 470010000, 470020000},
		NodeLon:   []int32{110000000, 110000000, 110000000},
		NodeEle:   []int32{1000, 2000, 3000},
		IsBorder:  make([]bool, 3),
		NodeTags:  make([]byte, 3),
	}
	g.BuildReverseAdj()
	reader := &SingleGraphReader{G: g}

	// Node 0: no incoming edges
	var count0 int
	reader.ForEachInEdge(MakeGlobalNodeID(0, 0), func(ev EdgeView) { count0++ })
	if count0 != 0 {
		t.Errorf("node 0 should have 0 incoming edges, got %d", count0)
	}

	// Node 1: 1 incoming edge (from 0, dist=1000)
	var count1 int
	var inDist1 uint32
	reader.ForEachInEdge(MakeGlobalNodeID(0, 1), func(ev EdgeView) {
		count1++
		inDist1 = ev.Dist
	})
	if count1 != 1 {
		t.Errorf("node 1 should have 1 incoming edge, got %d", count1)
	}
	if inDist1 != 1000 {
		t.Errorf("incoming edge dist = %d, want 1000", inDist1)
	}

	// Node 2: 1 incoming edge (from 1, dist=2000)
	var count2 int
	var inDist2 uint32
	reader.ForEachInEdge(MakeGlobalNodeID(0, 2), func(ev EdgeView) {
		count2++
		inDist2 = ev.Dist
	})
	if count2 != 1 {
		t.Errorf("node 2 should have 1 incoming edge, got %d", count2)
	}
	if inDist2 != 2000 {
		t.Errorf("incoming edge dist = %d, want 2000", inDist2)
	}
}

func TestShouldPruneStatic(t *testing.T) {
	br := &BackwardResult{
		LowerBound: map[uint64]float32{1: 10.0, 2: 20.0},
		UpperBound: 25.0,
	}
	// fwd=10 + bwd=10 = 20 < 25 → don't prune
	if br.ShouldPrune(1, 10.0) {
		t.Error("should not prune: 10+10=20 < 25")
	}
	// fwd=20 + bwd=20 = 40 > 25 → prune
	if !br.ShouldPrune(2, 20.0) {
		t.Error("should prune: 20+20=40 > 25")
	}
	// unknown node → don't prune
	if br.ShouldPrune(99, 100.0) {
		t.Error("should not prune unknown node")
	}
	// nil BackwardResult → don't prune
	var nilBR *BackwardResult
	if nilBR.ShouldPrune(1, 10.0) {
		t.Error("nil should not prune")
	}
}
