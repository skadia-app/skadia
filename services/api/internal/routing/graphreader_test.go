package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestMakeAndSplitGlobalNodeID(t *testing.T) {
	tileID := uint32(49146)
	localID := uint32(12345)
	gid := MakeGlobalNodeID(tileID, localID)
	gotTile, gotLocal := SplitGlobalNodeID(gid)
	if gotTile != tileID || gotLocal != localID {
		t.Errorf("roundtrip failed: (%d, %d) → %d → (%d, %d)", tileID, localID, gid, gotTile, gotLocal)
	}
}

func TestSingleGraphReaderForEachEdge(t *testing.T) {
	g := makeSimpleTestGraph() // helper that builds a 3-node, 2-edge graph
	reader := &SingleGraphReader{G: g, TileID: 0}

	var edges []EdgeView
	reader.ForEachEdge(MakeGlobalNodeID(0, 0), func(ev EdgeView) {
		edges = append(edges, ev)
	})
	if len(edges) == 0 {
		t.Fatal("expected edges from node 0")
	}
	// Check target is valid global ID
	_, localID := SplitGlobalNodeID(edges[0].Target)
	if localID >= g.NodeCount {
		t.Errorf("target local ID %d out of range", localID)
	}
}

func TestSingleGraphReaderNearestNode(t *testing.T) {
	g := makeSimpleTestGraph()
	reader := &SingleGraphReader{G: g, TileID: 0}
	nodeID, dist, ok := reader.NearestNode(47.05, 8.05)
	if !ok {
		t.Fatal("expected nearest node found")
	}
	if dist > 100000 { // 100km — sanity
		t.Errorf("distance too large: %f", dist)
	}
	_, localID := SplitGlobalNodeID(nodeID)
	if localID >= g.NodeCount {
		t.Errorf("local ID %d out of range", localID)
	}
}

func makeSimpleTestGraph() *graph.Graph {
	g := &graph.Graph{
		NodeCount: 3, EdgeCount: 2,
		Offsets:         []uint32{0, 1, 2, 2},
		Targets:         []uint32{1, 2},
		EdgeDist:        []uint32{10000, 20000},
		EdgeTags1:       []uint64{0, 0},
		EdgeTags2:       []uint64{0, 0},
		EdgeOSMIdx:      []uint32{0, 0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470500000, 470600000, 470700000},
		NodeLon:         []int32{80500000, 80600000, 80700000},
		NodeEle:         []int32{5000, 5100, 5200},
		NodeTags:        []uint8{0, 0, 0},
		EdgeGeomOff:     []uint32{0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false, false},
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	return g
}
