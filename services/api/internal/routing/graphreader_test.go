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

func TestSingleGraphReaderEdgeElevProfile_MultiEdge(t *testing.T) {
	// Graph with 3 edges of DIFFERENT elevation point counts:
	// Edge 0: 5 elev points → 4 segDists
	// Edge 1: 3 elev points → 2 segDists
	// Edge 2: 7 elev points → 6 segDists
	// Total: 15 elev points, 12 segDists
	// C05 offset: edge K → segDistStart = elevStart - K
	//   Edge 0: elevStart=0,  segDistStart=0-0=0, count=4
	//   Edge 1: elevStart=5,  segDistStart=5-1=4, count=2
	//   Edge 2: elevStart=8,  segDistStart=8-2=6, count=6
	g := &graph.Graph{
		NodeCount:       4,
		EdgeCount:       3,
		Offsets:         []uint32{0, 1, 2, 3, 3},
		Targets:         []uint32{1, 2, 3},
		EdgeDist:        []uint32{10000, 10000, 10000},
		EdgeTags1:       []uint64{0, 0, 0},
		EdgeTags2:       []uint64{0, 0, 0},
		EdgeOSMIdx:      []uint32{0, 0, 0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470000000, 470100000, 470200000, 470300000},
		NodeLon:         []int32{80000000, 80000000, 80000000, 80000000},
		NodeEle:         []int32{5000, 5100, 5200, 5300},
		NodeTags:        []uint8{0, 0, 0, 0},
		EdgeGeomOff:     []uint32{0, 0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 5, 8, 15},
		EdgeElevProf:    []int32{5000, 5020, 5050, 5080, 5100, 5100, 5150, 5200, 5200, 5210, 5230, 5250, 5270, 5290, 5300},
		EdgeElevSegDist: []uint16{100, 110, 105, 115, 120, 130, 90, 95, 100, 105, 110, 115},
		IsBorder:        []bool{false, false, false, false},
	}

	reader := &SingleGraphReader{G: g, TileID: 0}

	// Edge 0: 5 points → 4 segDists starting at offset 0 (segDistStart = 0 - 0 = 0)
	elevs0, segDists0 := reader.EdgeElevProfile(MakeGlobalNodeID(0, 0))
	if len(elevs0) != 5 {
		t.Errorf("edge 0: expected 5 elev points, got %d", len(elevs0))
	}
	if len(segDists0) != 4 {
		t.Errorf("edge 0: expected 4 segDists, got %d", len(segDists0))
	}
	if len(segDists0) > 0 && segDists0[0] != 100 {
		t.Errorf("edge 0 segDist[0]: expected 100, got %d", segDists0[0])
	}

	// Edge 1: 3 points → 2 segDists starting at offset 4 (segDistStart = 5 - 1 = 4)
	elevs1, segDists1 := reader.EdgeElevProfile(MakeGlobalNodeID(0, 1))
	if len(elevs1) != 3 {
		t.Errorf("edge 1: expected 3 elev points, got %d", len(elevs1))
	}
	if len(segDists1) != 2 {
		t.Errorf("edge 1: expected 2 segDists, got %d", len(segDists1))
	}
	if len(segDists1) > 0 && segDists1[0] != 120 {
		t.Errorf("edge 1 segDist[0]: expected 120, got %d", segDists1[0])
	}

	// Edge 2: 7 points → 6 segDists starting at offset 6 (segDistStart = 8 - 2 = 6)
	elevs2, segDists2 := reader.EdgeElevProfile(MakeGlobalNodeID(0, 2))
	if len(elevs2) != 7 {
		t.Errorf("edge 2: expected 7 elev points, got %d", len(elevs2))
	}
	if len(segDists2) != 6 {
		t.Errorf("edge 2: expected 6 segDists, got %d", len(segDists2))
	}
	if len(segDists2) > 0 && segDists2[0] != 90 {
		t.Errorf("edge 2 segDist[0]: expected 90, got %d", segDists2[0])
	}
}

func TestSingleGraphReaderEdgeGeometry_IntermediateOnly(t *testing.T) {
	// Edge with 5 geometry points: start + 3 intermediate + end
	g := &graph.Graph{
		NodeCount:       2,
		EdgeCount:       1,
		Offsets:         []uint32{0, 1, 1},
		Targets:         []uint32{1},
		EdgeDist:        []uint32{10000},
		EdgeTags1:       []uint64{0},
		EdgeTags2:       []uint64{0},
		EdgeOSMIdx:      []uint32{0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470000000, 471000000},
		NodeLon:         []int32{80000000, 81000000},
		NodeEle:         []int32{5000, 5100},
		NodeTags:        []uint8{0, 0},
		EdgeGeomOff:     []uint32{0, 5},
		EdgeGeomLat:     []int32{470000000, 470250000, 470500000, 470750000, 471000000},
		EdgeGeomLon:     []int32{80000000, 80250000, 80500000, 80750000, 81000000},
		EdgeElevOff:     []uint32{0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false},
	}

	reader := &SingleGraphReader{G: g, TileID: 0}

	lats, lons := reader.EdgeGeometry(MakeGlobalNodeID(0, 0))

	// Should return ONLY 3 intermediate points (excluding start/end)
	if len(lats) != 3 {
		t.Errorf("expected 3 intermediate points, got %d", len(lats))
	}
	if len(lons) != 3 {
		t.Errorf("expected 3 intermediate lon points, got %d", len(lons))
	}
	// First intermediate should NOT be start node coordinate
	if len(lats) > 0 && lats[0] == 470000000 {
		t.Error("first point should NOT be start node coordinate")
	}
	// Last intermediate should NOT be end node coordinate
	if len(lats) > 0 && lats[len(lats)-1] == 471000000 {
		t.Error("last point should NOT be end node coordinate")
	}
	// Verify actual intermediate values
	if len(lats) == 3 {
		if lats[0] != 470250000 {
			t.Errorf("intermediate[0] lat: expected 470250000, got %d", lats[0])
		}
		if lats[1] != 470500000 {
			t.Errorf("intermediate[1] lat: expected 470500000, got %d", lats[1])
		}
		if lats[2] != 470750000 {
			t.Errorf("intermediate[2] lat: expected 470750000, got %d", lats[2])
		}
	}

	// Edge with only 2 geom points (start+end, no intermediates) → expect nil/empty
	g2 := &graph.Graph{
		NodeCount:       2,
		EdgeCount:       1,
		Offsets:         []uint32{0, 1, 1},
		Targets:         []uint32{1},
		EdgeDist:        []uint32{10000},
		EdgeTags1:       []uint64{0},
		EdgeTags2:       []uint64{0},
		EdgeOSMIdx:      []uint32{0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470000000, 471000000},
		NodeLon:         []int32{80000000, 81000000},
		NodeEle:         []int32{5000, 5100},
		NodeTags:        []uint8{0, 0},
		EdgeGeomOff:     []uint32{0, 2},
		EdgeGeomLat:     []int32{470000000, 471000000},
		EdgeGeomLon:     []int32{80000000, 81000000},
		EdgeElevOff:     []uint32{0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false},
	}
	reader2 := &SingleGraphReader{G: g2, TileID: 0}
	lats2, _ := reader2.EdgeGeometry(MakeGlobalNodeID(0, 0))
	if len(lats2) != 0 {
		t.Errorf("edge with only start+end should return 0 intermediate points, got %d", len(lats2))
	}
}

func TestSingleGraphReaderFindNearestEdge(t *testing.T) {
	g := makePassableTestGraph()
	reader := &SingleGraphReader{G: g, TileID: 0}

	proj, ok := reader.FindNearestEdge(47.05, 8.05, nil)
	if !ok {
		t.Fatal("expected to find an edge")
	}
	if proj.DistanceM > 5000 {
		t.Errorf("distance too large: %.1f m", proj.DistanceM)
	}
	if proj.TileID != 0 {
		t.Errorf("expected tile 0, got %d", proj.TileID)
	}
}

func TestSingleGraphReaderFindNearestEdge_Filter(t *testing.T) {
	g := makePassableTestGraph()
	reader := &SingleGraphReader{G: g, TileID: 0}

	blockAll := func(tags1 uint64) bool { return false }
	_, ok := reader.FindNearestEdge(47.05, 8.05, blockAll)
	if ok {
		t.Error("expected no edge when filter blocks all")
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

func TestSingleGraphReader_EdgeTags2(t *testing.T) {
	g := &graph.Graph{
		NodeCount:       2,
		EdgeCount:       2,
		Offsets:         []uint32{0, 1, 2, 2},
		Targets:         []uint32{1, 0},
		EdgeDist:        []uint32{5000, 5000},
		EdgeTags1:       []uint64{0, 0},
		EdgeTags2:       []uint64{0xDEADBEEF00000001, 0xCAFEBABE00000002},
		EdgeOSMIdx:      []uint32{0, 0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470000000, 470100000},
		NodeLon:         []int32{80000000, 80100000},
		NodeEle:         []int32{5000, 5000},
		NodeTags:        []uint8{0, 0},
		EdgeGeomOff:     []uint32{0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false},
	}
	reader := &SingleGraphReader{G: g, TileID: 0}

	// Valid handle → edge 0: expect first Tags2 value.
	got0 := reader.EdgeTags2(MakeGlobalNodeID(0, 0))
	if got0 != 0xDEADBEEF00000001 {
		t.Errorf("edge 0 Tags2: expected 0xDEADBEEF00000001, got 0x%X", got0)
	}

	// Valid handle → edge 1: expect second Tags2 value.
	got1 := reader.EdgeTags2(MakeGlobalNodeID(0, 1))
	if got1 != 0xCAFEBABE00000002 {
		t.Errorf("edge 1 Tags2: expected 0xCAFEBABE00000002, got 0x%X", got1)
	}

	// Out-of-range edge index → expect 0.
	gotOOB := reader.EdgeTags2(MakeGlobalNodeID(0, 999))
	if gotOOB != 0 {
		t.Errorf("out-of-range handle: expected 0, got 0x%X", gotOOB)
	}
}

func TestSingleGraphReader_EdgeDistanceCm(t *testing.T) {
	g := &graph.Graph{
		NodeCount:       2,
		EdgeCount:       2,
		Offsets:         []uint32{0, 1, 2, 2},
		Targets:         []uint32{1, 0},
		EdgeDist:        []uint32{12345, 67890},
		EdgeTags1:       []uint64{0, 0},
		EdgeTags2:       []uint64{0, 0},
		EdgeOSMIdx:      []uint32{0, 0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470000000, 470100000},
		NodeLon:         []int32{80000000, 80100000},
		NodeEle:         []int32{5000, 5000},
		NodeTags:        []uint8{0, 0},
		EdgeGeomOff:     []uint32{0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false},
	}
	reader := &SingleGraphReader{G: g, TileID: 0}

	// Valid handle → edge 0.
	got0 := reader.EdgeDistanceCm(MakeGlobalNodeID(0, 0))
	if got0 != 12345 {
		t.Errorf("edge 0 DistCm: expected 12345, got %d", got0)
	}

	// Valid handle → edge 1.
	got1 := reader.EdgeDistanceCm(MakeGlobalNodeID(0, 1))
	if got1 != 67890 {
		t.Errorf("edge 1 DistCm: expected 67890, got %d", got1)
	}

	// Out-of-range edge index → expect 0.
	gotOOB := reader.EdgeDistanceCm(MakeGlobalNodeID(0, 999))
	if gotOOB != 0 {
		t.Errorf("out-of-range handle: expected 0, got %d", gotOOB)
	}
}
