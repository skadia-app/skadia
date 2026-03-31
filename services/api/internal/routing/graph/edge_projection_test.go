package graph

import (
	"testing"
)

func makeEdgeProjectionTestGraph() *Graph {
	const trackTags1 uint64 = 15 // highway=track
	g := &Graph{
		NodeCount: 4, EdgeCount: 3,
		Offsets:   []uint32{0, 1, 2, 3, 3},
		Targets:   []uint32{1, 2, 3},
		EdgeDist:  []uint32{100_00, 100_00, 100_00}, // 100m in cm
		EdgeTags1: []uint64{trackTags1, trackTags1, trackTags1},
		EdgeTags2: []uint64{0, 0, 0},
		NodeLat:   []int32{470000000, 470100000, 470200000, 470300000},
		NodeLon:   []int32{80000000, 80100000, 80200000, 80300000},
		NodeEle:   []int32{5000, 5100, 5200, 5300},
		NodeTags:  []uint8{0, 0, 0, 0},
		EdgeGeomOff: []uint32{0, 3, 3, 3, 3},
		EdgeGeomLat: []int32{470000000, 470050000, 470100000},
		EdgeGeomLon: []int32{80000000, 80050000, 80100000},
		EdgeElevOff: []uint32{0, 0, 0, 0},
		EdgeElevProf: []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder: []bool{false, false, false, false},
	}
	g.Grid = BuildSpatialIndex(g, 0.01)
	return g
}

func TestFindNearestEdgeLocal_Basic(t *testing.T) {
	g := makeEdgeProjectionTestGraph()
	proj, ok := g.FindNearestEdgeLocal(47.005, 8.005, nil)
	if !ok {
		t.Fatal("expected to find an edge projection")
	}
	if proj.DistanceM > 100 {
		t.Errorf("projection distance too large: %.1f m", proj.DistanceM)
	}
	if proj.FractionAB < 0.3 || proj.FractionAB > 0.7 {
		t.Errorf("expected fraction near 0.5, got %.3f", proj.FractionAB)
	}
	if proj.EdgeIdx > g.EdgeCount {
		t.Errorf("edge index out of range: %d", proj.EdgeIdx)
	}
	if proj.NodeA >= g.NodeCount || proj.NodeB >= g.NodeCount {
		t.Errorf("node IDs out of range: A=%d, B=%d", proj.NodeA, proj.NodeB)
	}
}

func TestFindNearestEdgeLocal_Filter(t *testing.T) {
	g := makeEdgeProjectionTestGraph()
	blockAll := func(tags1 uint64) bool { return false }
	_, ok := g.FindNearestEdgeLocal(47.005, 8.005, blockAll)
	if ok {
		t.Error("expected no edge when filter blocks everything")
	}
}

func TestFindNearestEdgeLocal_EarlyTermination(t *testing.T) {
	g := makeEdgeProjectionTestGraph()
	proj, ok := g.FindNearestEdgeLocal(47.0, 8.0, nil)
	if !ok {
		t.Fatal("expected to find an edge")
	}
	if proj.DistanceM > 10 {
		t.Errorf("expected very small distance at node, got %.1f m", proj.DistanceM)
	}
	if proj.FractionAB > 0.1 {
		t.Errorf("expected fraction near 0 at edge start, got %.3f", proj.FractionAB)
	}
}

func TestFindNearestEdgeLocal_ElevationInterpolation(t *testing.T) {
	g := makeEdgeProjectionTestGraph()
	proj, ok := g.FindNearestEdgeLocal(47.005, 8.005, nil)
	if !ok {
		t.Fatal("expected to find an edge")
	}
	if proj.ProjEleDm < 5020 || proj.ProjEleDm > 5080 {
		t.Errorf("expected interpolated elevation ~5050 dm, got %d", proj.ProjEleDm)
	}
}
