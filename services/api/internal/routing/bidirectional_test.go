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
	g := &graph.Graph{
		NodeCount: 5,
		EdgeCount: 8,
		Offsets:   []uint32{0, 1, 3, 5, 7, 8},
		Targets:   []uint32{1, 0, 2, 1, 3, 2, 4, 3},
		EdgeDist:  []uint32{1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000},
		EdgeTags1: make([]uint64, 8),
		EdgeTags2: make([]uint64, 8),
		NodeLat:   []int32{470000000, 470010000, 470020000, 470030000, 470040000},
		NodeLon:   []int32{110000000, 110000000, 110000000, 110000000, 110000000},
		NodeEle:   []int32{1000, 1000, 1000, 1000, 1000},
		IsBorder:  make([]bool, 5),
		NodeTags:  make([]byte, 5),
	}
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
		EdgeTags1: make([]uint64, 2),
		EdgeTags2: make([]uint64, 2),
		NodeLat:   []int32{470000000, 470010000, 470020000},
		NodeLon:   []int32{110000000, 110000000, 110000000},
		NodeEle:   []int32{1000, 1000, 1000},
		IsBorder:  make([]bool, 3),
		NodeTags:  make([]byte, 3),
	}
	reader := &SingleGraphReader{G: g}
	cfg := DefaultCostConfig()
	result := RunBackwardSearch(reader, 2, cfg, 100000)
	// Node 2 has no outgoing edges, so only itself is in LowerBound
	if len(result.LowerBound) != 1 {
		t.Errorf("expected 1 node, got %d", len(result.LowerBound))
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
