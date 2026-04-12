package routing

import (
	"context"
	"testing"
	"unsafe"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestParetoStateSize(t *testing.T) {
	size := unsafe.Sizeof(paretoState{})
	if size < 80 || size > 120 {
		t.Errorf("paretoState size = %d bytes, expected 80-120", size)
	}
	t.Logf("paretoState size: %d bytes", size)
}

func TestParetoStateDominance(t *testing.T) {
	a := paretoState{cost: 100, ascent: 50}
	b := paretoState{cost: 200, ascent: 100}
	if !a.dominates(b) {
		t.Error("a should dominate b")
	}
	if b.dominates(a) {
		t.Error("b should not dominate a")
	}

	c := paretoState{cost: 100, ascent: 50}
	if a.dominates(c) {
		t.Error("equal states should not dominate")
	}
	if c.dominates(a) {
		t.Error("equal states should not dominate (symmetric)")
	}

	d := paretoState{cost: 100, ascent: 100}
	e := paretoState{cost: 150, ascent: 50}
	if d.dominates(e) || e.dominates(d) {
		t.Error("Pareto-optimal pair should not dominate each other")
	}
}

func TestTryInsertDominated(t *testing.T) {
	rs := newReachableState()
	nodeID := uint64(42)

	_, ok := rs.tryInsert(nodeID, paretoState{cost: 100, ascent: 50})
	if !ok {
		t.Fatal("first insert should succeed")
	}

	_, ok = rs.tryInsert(nodeID, paretoState{cost: 200, ascent: 100})
	if ok {
		t.Error("dominated state should be rejected")
	}

	front := rs.fronts[nodeID]
	if len(front.activeIDs) != 1 {
		t.Errorf("front should have 1 active state, got %d", len(front.activeIDs))
	}
	if len(rs.slab.states) != 1 {
		t.Errorf("slab should have 1 state, got %d", len(rs.slab.states))
	}
}

func TestTryInsertRemovesDominated(t *testing.T) {
	rs := newReachableState()
	nodeID := uint64(42)

	_, _ = rs.tryInsert(nodeID, paretoState{cost: 100, ascent: 100})
	_, _ = rs.tryInsert(nodeID, paretoState{cost: 150, ascent: 50})

	front := rs.fronts[nodeID]
	if len(front.activeIDs) != 2 {
		t.Fatalf("after 2 incomparable inserts, front should have 2, got %d", len(front.activeIDs))
	}

	_, ok := rs.tryInsert(nodeID, paretoState{cost: 80, ascent: 40})
	if !ok {
		t.Fatal("dominant insert should succeed")
	}

	if len(front.activeIDs) != 1 {
		t.Errorf("after dominant insert, front should have 1, got %d", len(front.activeIDs))
	}
	if len(rs.slab.states) != 3 {
		t.Errorf("slab should have 3 states, got %d", len(rs.slab.states))
	}
}

func TestTryInsertSortedOrder(t *testing.T) {
	rs := newReachableState()
	nodeID := uint64(42)

	for _, cost := range []float32{300, 100, 200} {
		_, _ = rs.tryInsert(nodeID, paretoState{cost: cost, ascent: 1000 - cost})
	}

	front := rs.fronts[nodeID]
	if len(front.activeIDs) != 3 {
		t.Fatalf("expected 3 active states, got %d", len(front.activeIDs))
	}

	for i := 1; i < len(front.activeIDs); i++ {
		prev := rs.slab.states[front.activeIDs[i-1]]
		cur := rs.slab.states[front.activeIDs[i]]
		if prev.cost > cur.cost {
			t.Errorf("front not sorted: [%d].cost=%f > [%d].cost=%f",
				i-1, prev.cost, i, cur.cost)
		}
	}
}

func TestParetoFrontierHeapOrder(t *testing.T) {
	f := paretoFrontier{entries: make([]paretoFrontierEntry, 0, 16)}
	costs := []float32{50, 10, 30, 20, 40}
	for i, c := range costs {
		f.Push(paretoFrontierEntry{
			nodeID:  uint64(i),
			stateID: uint32(i),
			cost:    c,
		})
	}
	prev := float32(0)
	for f.Len() > 0 {
		e := f.Pop()
		if e.cost < prev {
			t.Errorf("heap order violated: %f < %f", e.cost, prev)
		}
		prev = e.cost
	}
}

// --- tinyTestReader: minimal GraphReader mock for Pareto integration tests ---

// tinyTestReader is a hand-built 3-node bidirectional graph:
//
//	Node 0 (src): lat=47.0°, lon=11.0°, ele=5000 dm
//	Node 1:       lat=47.005°, lon=11.005°, ele=6000 dm (+100 m ascent)
//	Node 2:       lat=47.01°, lon=11.0°, ele=5000 dm
//
// Edges (bidirectional):
//
//	0↔1: 700 m, +100 m ascent (0→1), tags=path (hw=16)
//	1↔2: 700 m, -100 m descent (1→2), tags=track (hw=15)
//	0↔2: 1000 m, flat, tags=residential (hw=11)
type tinyTestReader struct{}

// tags1 helpers: highway enum is bits 0-4 of tags1.
// path=16, track=15, residential=11 — all accessible under "hiking".
const (
	tinyTags1Path        = uint64(16) // path
	tinyTags1Track       = uint64(15) // track
	tinyTags1Residential = uint64(11) // residential
)

// nodeCoords holds lat, lon, ele for each node.
var tinyNodes = [3]struct{ lat, lon, ele int32 }{
	{lat: 470000000, lon: 110000000, ele: 5000}, // node 0
	{lat: 470050000, lon: 110050000, ele: 6000}, // node 1
	{lat: 470100000, lon: 110000000, ele: 5000}, // node 2
}

// tinyEdges maps nodeID → outgoing EdgeViews.
var tinyEdges = map[uint64][]EdgeView{
	0: {
		{Target: 1, TargetLat: tinyNodes[1].lat, TargetLon: tinyNodes[1].lon,
			Dist: 70000, Tags1: tinyTags1Path, DeltaEle: +1000},
		{Target: 2, TargetLat: tinyNodes[2].lat, TargetLon: tinyNodes[2].lon,
			Dist: 100000, Tags1: tinyTags1Residential, DeltaEle: 0},
	},
	1: {
		{Target: 0, TargetLat: tinyNodes[0].lat, TargetLon: tinyNodes[0].lon,
			Dist: 70000, Tags1: tinyTags1Path, DeltaEle: -1000},
		{Target: 2, TargetLat: tinyNodes[2].lat, TargetLon: tinyNodes[2].lon,
			Dist: 70000, Tags1: tinyTags1Track, DeltaEle: -1000},
	},
	2: {
		{Target: 1, TargetLat: tinyNodes[1].lat, TargetLon: tinyNodes[1].lon,
			Dist: 70000, Tags1: tinyTags1Track, DeltaEle: +1000},
		{Target: 0, TargetLat: tinyNodes[0].lat, TargetLon: tinyNodes[0].lon,
			Dist: 100000, Tags1: tinyTags1Residential, DeltaEle: 0},
	},
}

func (r *tinyTestReader) ForEachEdge(nodeID uint64, fn func(EdgeView)) {
	for _, ev := range tinyEdges[nodeID] {
		fn(ev)
	}
}

func (r *tinyTestReader) NodeCoord(nodeID uint64) (lat, lon, ele int32) {
	if int(nodeID) < len(tinyNodes) {
		n := tinyNodes[nodeID]
		return n.lat, n.lon, n.ele
	}
	return 0, 0, 0
}

// All remaining GraphReader methods are unused by findReachable — panic stubs.

func (r *tinyTestReader) ForEachInEdge(_ uint64, _ func(EdgeView)) {
	panic("tinyTestReader: ForEachInEdge not implemented")
}

func (r *tinyTestReader) NearestNode(_, _ float64) (uint64, float64, bool) {
	panic("tinyTestReader: NearestNode not implemented")
}

func (r *tinyTestReader) FindNearestEdge(_, _ float64, _ func(uint64) bool) (graph.EdgeProjection, bool) {
	panic("tinyTestReader: FindNearestEdge not implemented")
}

func (r *tinyTestReader) EdgeElevProfile(_ uint64) ([]int32, []uint16) {
	panic("tinyTestReader: EdgeElevProfile not implemented")
}

func (r *tinyTestReader) EdgeGeometry(_ uint64) ([]int32, []int32) {
	panic("tinyTestReader: EdgeGeometry not implemented")
}

func (r *tinyTestReader) EdgeOSMID(_ uint64) int64 {
	panic("tinyTestReader: EdgeOSMID not implemented")
}

func (r *tinyTestReader) LandmarkDists(_ LandmarkSetID, _ uint64) []float32 {
	return nil
}

func (r *tinyTestReader) LandmarkDistsInto(_ LandmarkSetID, _ uint64, _ []float32, _ []byte) []float32 {
	return nil
}

func (r *tinyTestReader) LandmarkDistsPageInto(_ LandmarkSetID, _ uint32, _, _ uint32, _ []float32, _ []byte) []float32 {
	return nil
}

func (r *tinyTestReader) LandmarkMeta() *graph.LandmarkMetaData {
	return nil
}

func (r *tinyTestReader) GraphAt(_, _ float64) *graph.Graph {
	return nil
}

func (r *tinyTestReader) HasTilesFor(_, _, _, _ float64) bool {
	return true
}

// --- Integration test ---

func TestFindReachablePareto_SmallGraph(t *testing.T) {
	reader := &tinyTestReader{}
	cfg, err := BuildCostConfig(ProfileConfig{Profile: "hiking"})
	if err != nil {
		t.Fatalf("BuildCostConfig: %v", err)
	}

	ctx := context.Background()
	rs, err := findReachable(ctx, reader, 0, cfg, 50000)
	if err != nil {
		t.Fatalf("findReachable: %v", err)
	}

	// Verify we reached at least 3 nodes (0, 1, 2).
	if len(rs.fronts) < 3 {
		t.Errorf("expected at least 3 reachable nodes, got %d", len(rs.fronts))
	}

	// Verify not truncated on tiny graph.
	if rs.truncated {
		t.Errorf("should not be truncated on small graph (reason: %q)", rs.truncatedReason)
	}

	// Verify all fronts have at least 1 active state.
	for nodeID, front := range rs.fronts {
		if len(front.activeIDs) == 0 {
			t.Errorf("node %d has empty front", nodeID)
		}
	}

	// Verify source node (0) has cost 0.
	if front, ok := rs.fronts[0]; ok && len(front.activeIDs) > 0 {
		srcState := rs.slab.states[front.activeIDs[0]]
		if srcState.cost != 0 {
			t.Errorf("source node cost = %v, expected 0", srcState.cost)
		}
	} else {
		t.Error("source node 0 not found in fronts")
	}

	t.Logf("reachable nodes: %d, total states in slab: %d", len(rs.fronts), len(rs.slab.states))
}
