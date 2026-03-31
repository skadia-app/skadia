package routing

import (
	"context"
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

// TestSameEdgeRoute_Reversed tests B1 fix: when src is at the high end and dst
// is at the low end of the same edge (reversed travel direction), AscentM should
// be ~0 and DescentM should be > 0 (going downhill).
func TestSameEdgeRoute_Reversed(t *testing.T) {
	const trackTags1 uint64 = 15
	// 2-node bidirectional graph: Node 0 at 500m, Node 1 at 600m.
	// Both directions exist: edge 0 = 0→1, edge 1 = 1→0.
	g := &graph.Graph{
		NodeCount: 2, EdgeCount: 2,
		Offsets:         []uint32{0, 1, 2},
		Targets:         []uint32{1, 0},
		EdgeDist:        []uint32{100_000, 100_000}, // 1km in cm
		EdgeTags1:       []uint64{trackTags1, trackTags1},
		EdgeTags2:       []uint64{0, 0},
		EdgeOSMIdx:      []uint32{0, 0},
		OSMIDPool:       []int64{1},
		// Node 0 at lat 47.00, Node 1 at lat 47.01 — ~1km apart
		NodeLat:         []int32{470000000, 470100000},
		NodeLon:         []int32{80000000, 80000000},
		// Node 0 at 500m (5000dm), Node 1 at 600m (6000dm)
		NodeEle:         []int32{5000, 6000},
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

	// src near Node 1 (high, lat ~47.009), dst near Node 0 (low, lat ~47.001)
	// This is the reversed case: traveling downhill on the same edge.
	srcVE, err := r.snapToEdge(47.009, 8.0, cfg)
	if err != nil {
		t.Fatalf("snap src (high end): %v", err)
	}
	dstVE, err := r.snapToEdge(47.001, 8.0, cfg)
	if err != nil {
		t.Fatalf("snap dst (low end): %v", err)
	}

	if srcVE.Proj.EdgeIdx != dstVE.Proj.EdgeIdx || srcVE.Proj.TileID != dstVE.Proj.TileID {
		t.Skip("projections landed on different edges — not a same-edge test")
	}

	// Verify src is at higher fraction (closer to Node 1) than dst (closer to Node 0)
	if srcVE.Proj.FractionAB <= dstVE.Proj.FractionAB {
		t.Skipf("src fraction (%.3f) not > dst fraction (%.3f) — not the reversed case",
			srcVE.Proj.FractionAB, dstVE.Proj.FractionAB)
	}

	result := r.routeSameEdge(srcVE, dstVE, cfg)
	if result == nil {
		t.Fatal("routeSameEdge returned nil")
	}
	if result.DistanceM <= 0 {
		t.Errorf("expected positive distance, got %.2f", result.DistanceM)
	}
	// Going downhill: AscentM should be ~0, DescentM should be > 0
	if result.AscentM > 1.0 {
		t.Errorf("reversed (downhill) route: AscentM should be ~0, got %.2f", result.AscentM)
	}
	if result.DescentM <= 0 {
		t.Errorf("reversed (downhill) route: DescentM should be > 0, got %.2f", result.DescentM)
	}
}

// TestFindRouteUnified_VirtualEndpoints tests dual-VE termination: routing with
// both srcVE and dstVE set should produce a valid result reaching one of dstVE's nodes.
func TestFindRouteUnified_VirtualEndpoints(t *testing.T) {
	g := makeChainGraph(5) // 5 nodes: 0→1→2→3→4
	reader := &SingleGraphReader{G: g, TileID: 0}
	r := &UnifiedRouter{reader: reader, sem: make(chan struct{}, 1)}
	cfg := DefaultCostConfig()

	// Snap src near node 0, dst near node 4
	srcVE, err := r.snapToEdge(47.0001, 8.0, cfg)
	if err != nil {
		t.Fatalf("snap src: %v", err)
	}
	dstVE, err := r.snapToEdge(47.0361, 8.0, cfg) // node 4 at lat 47.000+4*0.009 = 47.036
	if err != nil {
		t.Fatalf("snap dst: %v", err)
	}

	// Pass0 with both VEs
	res0, err := findRouteUnified(context.Background(), reader, srcVE, dstVE, 0, 0,
		cfg, cfg.Pass0Coefficient, 0, nil, nil, nil)
	if err != nil {
		t.Fatalf("pass0 with VEs failed: %v", err)
	}
	if res0.totalCost <= 0 {
		t.Errorf("expected totalCost > 0, got %f", res0.totalCost)
	}
	// bestDstNode must be one of dstVE's nodes
	if res0.bestDstNode != dstVE.GlobalA && res0.bestDstNode != dstVE.GlobalB {
		t.Errorf("bestDstNode %d is neither dstVE.GlobalA (%d) nor dstVE.GlobalB (%d)",
			res0.bestDstNode, dstVE.GlobalA, dstVE.GlobalB)
	}
}

// TestRetrackUnified_VirtualEndpoints tests the full routing pipeline with VEs:
// pass0 → pass1 → retrack. The first coordinate should be near the srcVE projection
// and the last near the dstVE projection.
func TestRetrackUnified_VirtualEndpoints(t *testing.T) {
	g := makeChainGraph(5) // 5 nodes: 0→1→2→3→4
	reader := &SingleGraphReader{G: g, TileID: 0}
	r := &UnifiedRouter{reader: reader, sem: make(chan struct{}, 1)}
	cfg := DefaultCostConfig()

	// Snap src near node 0, dst near node 4
	srcVE, err := r.snapToEdge(47.0001, 8.0, cfg)
	if err != nil {
		t.Fatalf("snap src: %v", err)
	}
	dstVE, err := r.snapToEdge(47.0361, 8.0, cfg)
	if err != nil {
		t.Fatalf("snap dst: %v", err)
	}

	// Pass0
	res0, err := findRouteUnified(context.Background(), reader, srcVE, dstVE, 0, 0,
		cfg, cfg.Pass0Coefficient, 0, nil, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}

	// Pass1
	trackIndex := buildTrackIndexUnified(res0.state, res0.bestDstNode, 0)
	pass1MaxCost := res0.totalCost * cfg.Pass0Coefficient * cfg.Pass0Coefficient
	res1, err := findRouteUnified(context.Background(), reader, srcVE, dstVE, 0, 0,
		cfg, 0, pass1MaxCost, trackIndex, nil, nil)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	// Retrack with VEs
	result := retrackUnified(reader, res1.state, res1.bestDstNode, srcVE, dstVE)
	if result == nil {
		t.Fatal("retrackUnified with VEs returned nil")
	}
	if len(result.Coordinates) < 2 {
		t.Errorf("expected >= 2 coordinates, got %d", len(result.Coordinates))
	}

	// First coordinate should be near srcVE projection
	first := result.Coordinates[0]
	srcLat, srcLon := srcVE.Proj.ProjLat, srcVE.Proj.ProjLon
	if abs64(first.Lat-srcLat) > 0.01 || abs64(first.Lon-srcLon) > 0.01 {
		t.Errorf("first coord (%.6f, %.6f) not near srcVE (%.6f, %.6f)",
			first.Lat, first.Lon, srcLat, srcLon)
	}

	// Last coordinate should be near dstVE projection
	last := result.Coordinates[len(result.Coordinates)-1]
	dstLat, dstLon := dstVE.Proj.ProjLat, dstVE.Proj.ProjLon
	if abs64(last.Lat-dstLat) > 0.01 || abs64(last.Lon-dstLon) > 0.01 {
		t.Errorf("last coord (%.6f, %.6f) not near dstVE (%.6f, %.6f)",
			last.Lat, last.Lon, dstLat, dstLon)
	}
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
