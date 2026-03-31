package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestSnapToEdge_Basic(t *testing.T) {
	g := makePassableTestGraph()
	reader := &SingleGraphReader{G: g, TileID: 0}
	r := &UnifiedRouter{reader: reader, sem: make(chan struct{}, 1)}
	cfg := DefaultCostConfig()

	ve, err := r.snapToEdge(47.05, 8.05, cfg)
	if err != nil {
		t.Fatalf("snapToEdge failed: %v", err)
	}
	if ve == nil {
		t.Fatal("expected non-nil VirtualEndpoint")
	}
	if ve.GlobalA == 0 && ve.GlobalB == 0 {
		t.Error("expected non-zero global node IDs")
	}
	if ve.CostA < 0 || ve.CostB < 0 {
		t.Errorf("costs should be >= 0: CostA=%.4f, CostB=%.4f", ve.CostA, ve.CostB)
	}
	if ve.StateA.BestCost != ve.CostA {
		t.Errorf("StateA.BestCost (%.4f) != CostA (%.4f)", ve.StateA.BestCost, ve.CostA)
	}
	if ve.StateB.BestCost != ve.CostB {
		t.Errorf("StateB.BestCost (%.4f) != CostB (%.4f)", ve.StateB.BestCost, ve.CostB)
	}
}

func TestSnapToEdge_ProfileFilter(t *testing.T) {
	g := makePassableTestGraph()
	// Set all edges to motorway (highway=1) which is blocked by trekking profile.
	for i := range g.EdgeTags1 {
		g.EdgeTags1[i] = 1 // motorway
	}
	reader := &SingleGraphReader{G: g, TileID: 0}
	r := &UnifiedRouter{reader: reader, sem: make(chan struct{}, 1)}
	cfg := DefaultCostConfig()

	_, err := r.snapToEdge(47.05, 8.05, cfg)
	if err == nil {
		t.Error("expected ErrNoRoute when all edges are blocked")
	}
}

func TestSameEdgeRoute(t *testing.T) {
	const trackTags1 uint64 = 15
	g := &graph.Graph{
		NodeCount: 2, EdgeCount: 2,
		Offsets:         []uint32{0, 1, 2},
		Targets:         []uint32{1, 0},
		EdgeDist:        []uint32{100_000, 100_000}, // 1km
		EdgeTags1:       []uint64{trackTags1, trackTags1},
		EdgeTags2:       []uint64{0, 0},
		EdgeOSMIdx:      []uint32{0, 0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470000000, 470100000},
		NodeLon:         []int32{80000000, 80000000},
		NodeEle:         []int32{5000, 5100},
		NodeTags:        []uint8{0, 0},
		EdgeGeomOff:     []uint32{0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false},
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	reader := &SingleGraphReader{G: g, TileID: 0}
	r := &UnifiedRouter{reader: reader, sem: make(chan struct{}, 1)}
	cfg := DefaultCostConfig()

	srcVE, err := r.snapToEdge(47.002, 8.0, cfg)
	if err != nil {
		t.Fatalf("snap src: %v", err)
	}
	dstVE, err := r.snapToEdge(47.008, 8.0, cfg)
	if err != nil {
		t.Fatalf("snap dst: %v", err)
	}

	if srcVE.Proj.EdgeIdx != dstVE.Proj.EdgeIdx || srcVE.Proj.TileID != dstVE.Proj.TileID {
		t.Skip("projections landed on different edges — not a same-edge test")
	}

	result := r.routeSameEdge(srcVE, dstVE, cfg)
	if result == nil {
		t.Fatal("routeSameEdge returned nil")
	}
	if result.DistanceM <= 0 {
		t.Error("expected positive distance")
	}
	if len(result.Coordinates) < 2 {
		t.Error("expected at least 2 coordinates")
	}
}
