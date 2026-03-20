package routing

import (
	"context"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// makePassableTestGraph returns a 3-node, 2-edge graph where edges have
// highway=track (value 15, bits 0-4 of Tags1) so they pass cost config checks.
func makePassableTestGraph() *graph.Graph {
	// Highway=track is enum value 15 (0x0F), stored in Tags1 bits 0-4.
	// This is passable for all foot/bike profiles (only hw <= 4 or hw == 0 is blocked).
	const trackTags1 uint64 = 15 // highway=track
	g := &graph.Graph{
		NodeCount: 3, EdgeCount: 2,
		Offsets:         []uint32{0, 1, 2, 2},
		Targets:         []uint32{1, 2},
		EdgeDist:        []uint32{10000, 20000}, // 100m, 200m
		EdgeTags1:       []uint64{trackTags1, trackTags1},
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

// makeChainGraph creates a graph with n nodes in a chain (0→1→2→...→n-1).
// Each edge is 100m (10000 cm), with 10m ascending elevation per node (~1m/node).
// highway=track (enum 15) ensures edges are passable for all profiles.
func makeChainGraph(n int) *graph.Graph {
	if n < 2 {
		panic("makeChainGraph: need at least 2 nodes")
	}
	edgeCount := uint32(n - 1)
	const trackTags1 uint64 = 15 // highway=track, bits 0-4

	offsets := make([]uint32, n+1)
	targets := make([]uint32, edgeCount)
	dist := make([]uint32, edgeCount)
	tags1 := make([]uint64, edgeCount)
	tags2 := make([]uint64, edgeCount)
	osmIdx := make([]uint32, edgeCount)
	// EdgeGeomOff and EdgeElevOff need EdgeCount+1 entries
	geomOff := make([]uint32, edgeCount+1)
	elevOff := make([]uint32, edgeCount+1)
	nodeLat := make([]int32, n)
	nodeLon := make([]int32, n)
	nodeEle := make([]int32, n)
	nodeTags := make([]uint8, n)
	isBorder := make([]bool, n)

	for i := range n {
		// Space nodes ~100m apart in latitude (~0.0009° per 100m)
		nodeLat[i] = int32(470000000 + i*9000)
		nodeLon[i] = int32(80000000)
		// 10m ascending per node (in decimeters: 100 dm per node)
		nodeEle[i] = int32(5000 + i*100)
	}

	// offsets[i] = start index of node i's edges in Targets.
	// Each node 0..n-2 has exactly one outgoing edge (edge i).
	// Node n-1 (last node) has no outgoing edges.
	// So offsets = [0, 1, 2, ..., n-1, n-1] (length n+1).
	for i := range n - 1 {
		offsets[i] = uint32(i)
		targets[i] = uint32(i + 1)
		dist[i] = 10000 // 100m in centimeters
		tags1[i] = trackTags1
		// geomOff and elevOff remain 0 (no intermediate geometry/elevation points)
	}
	// Last node (n-1) has no outgoing edges
	offsets[n-1] = edgeCount
	offsets[n] = edgeCount
	// geomOff[edgeCount] and elevOff[edgeCount] remain 0 — all ranges are empty

	g := &graph.Graph{
		NodeCount:       uint32(n),
		EdgeCount:       edgeCount,
		Offsets:         offsets,
		Targets:         targets,
		EdgeDist:        dist,
		EdgeTags1:       tags1,
		EdgeTags2:       tags2,
		EdgeOSMIdx:      osmIdx,
		OSMIDPool:       []int64{1},
		EdgeGeomOff:     geomOff,
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     elevOff,
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		NodeLat:         nodeLat,
		NodeLon:         nodeLon,
		NodeEle:         nodeEle,
		NodeTags:        nodeTags,
		IsBorder:        isBorder,
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	return g
}

// TestFindRouteUnified_MultiEdgePath verifies that findRouteUnified correctly
// routes through a 5-node chain graph and that all intermediate nodes appear
// in the parent chain, and the distance is consistent with edge distances.
func TestFindRouteUnified_MultiEdgePath(t *testing.T) {
	g := makeChainGraph(5) // nodes 0–4, edges 0→1→2→3→4
	reader := &SingleGraphReader{G: g, TileID: 0}
	cfg := DefaultCostConfig()

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 4)

	// Pass0
	pass0, err := findRouteUnified(context.Background(), reader, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}
	if pass0.totalCost <= 0 {
		t.Errorf("pass0 totalCost should be > 0, got %f", pass0.totalCost)
	}

	trackIndex := buildTrackIndexUnified(pass0.state, src, dst)
	if trackIndex == nil {
		t.Fatal("trackIndex should not be nil")
	}

	// Pass1
	pass1, err := findRouteUnified(context.Background(), reader, src, dst, cfg, 0, pass0.totalCost, trackIndex, nil)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	// Verify all intermediate nodes are in the parent chain
	// Walk chain from dst back to src: should pass through nodes 3, 2, 1 → 0
	visited := make(map[uint64]bool)
	cur := dst
	for cur != src {
		visited[cur] = true
		ns, ok := pass1.state.nodes[cur]
		if !ok {
			t.Fatalf("node %d missing from state during chain walk", cur)
		}
		cur = ns.Parent
	}
	visited[src] = true

	// All 5 nodes must be visited
	for localID := uint32(0); localID < 5; localID++ {
		gid := MakeGlobalNodeID(0, localID)
		if !visited[gid] {
			t.Errorf("node %d not in parent chain", localID)
		}
	}

	// Retrack and check distance: 4 edges × 100m = 400m (haversine may differ slightly)
	result := retrackUnified(reader, pass1.state, src, dst)
	if result == nil {
		t.Fatal("retrack returned nil")
	}
	if len(result.Coordinates) < 5 {
		t.Errorf("expected >= 5 coordinates (one per node), got %d", len(result.Coordinates))
	}
	// Distance should be roughly 400m (4 × 100m); allow ±20% for projection differences
	const expectedDistM = 400.0
	if result.DistanceM < expectedDistM*0.8 || result.DistanceM > expectedDistM*1.2 {
		t.Errorf("expected distance ~400m, got %.1fm", result.DistanceM)
	}
}

// TestFindRouteUnified_NoRoute verifies that routing between two disconnected nodes
// returns an error containing "no route found".
func TestFindRouteUnified_NoRoute(t *testing.T) {
	// Build a graph with 2 isolated nodes and no edges.
	// Node 0 and node 1 are disconnected: neither has outgoing edges.
	g := &graph.Graph{
		NodeCount: 2, EdgeCount: 0,
		Offsets:         []uint32{0, 0, 0}, // both nodes have 0 edges
		Targets:         []uint32{},
		EdgeDist:        []uint32{},
		EdgeTags1:       []uint64{},
		EdgeTags2:       []uint64{},
		EdgeOSMIdx:      []uint32{},
		OSMIDPool:       []int64{},
		NodeLat:         []int32{470000000, 470900000},
		NodeLon:         []int32{80000000, 80000000},
		NodeEle:         []int32{5000, 5000},
		NodeTags:        []uint8{0, 0},
		EdgeGeomOff:     []uint32{0}, // EdgeCount+1 = 0+1 = 1 entry
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false},
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	reader := &SingleGraphReader{G: g, TileID: 0}

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 1)
	cfg := DefaultCostConfig()

	_, err := findRouteUnified(context.Background(), reader, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil)
	if err == nil {
		t.Fatal("expected error for disconnected nodes, got nil")
	}
	if !strings.Contains(err.Error(), "no route found") {
		t.Errorf("expected error containing 'no route found', got: %v", err)
	}
}

// TestRetrackUnified_CoordinateCount verifies that retrack over a 3-edge chain
// produces exactly 4 coordinates (one per node) when edges have no intermediate
// geometry points.
func TestRetrackUnified_CoordinateCount(t *testing.T) {
	g := makeChainGraph(4) // 4 nodes, 3 edges: 0→1→2→3
	reader := &SingleGraphReader{G: g, TileID: 0}
	cfg := DefaultCostConfig()

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 3)

	pass0, err := findRouteUnified(context.Background(), reader, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}
	trackIndex := buildTrackIndexUnified(pass0.state, src, dst)

	pass1, err := findRouteUnified(context.Background(), reader, src, dst, cfg, 0, pass0.totalCost, trackIndex, nil)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	result := retrackUnified(reader, pass1.state, src, dst)
	if result == nil {
		t.Fatal("retrack returned nil")
	}

	// 4 nodes → exactly 4 coordinates (no intermediate geometry in makeChainGraph)
	// EdgeGeometry returns nil when ≤2 geom points, so no DP intermediates are added.
	wantCoords := 4
	if len(result.Coordinates) != wantCoords {
		t.Errorf("expected exactly %d coordinates (4 nodes, no intermediate geom points), got %d",
			wantCoords, len(result.Coordinates))
	}

	// No duplicate consecutive coordinates (lat/lon equality)
	for i := 1; i < len(result.Coordinates); i++ {
		prev, cur := result.Coordinates[i-1], result.Coordinates[i]
		if prev.Lat == cur.Lat && prev.Lon == cur.Lon {
			t.Errorf("duplicate coordinate at index %d and %d: {%.7f, %.7f}", i-1, i, cur.Lat, cur.Lon)
		}
	}
}

// TestRetrackUnified_AscentDescent verifies that a route with strictly ascending
// elevation produces AscentM > 0 and DescentM == 0.
// makeChainGraph creates ascending nodes (+10m per node), but elevation is encoded
// in NodeEle (used for DeltaEle). Full elevation profile is empty (EdgeElevProf=[]),
// so ascent/descent comes from the per-edge NodeEle delta via EdgeElevProfile fallback.
// We use a custom graph with explicit elevation profiles to guarantee ascent tracking.
func TestRetrackUnified_AscentDescent(t *testing.T) {
	// Build a 3-node ascending graph with explicit elevation profile per edge.
	// Node 0: 500m, Node 1: 510m, Node 2: 520m (ascending).
	// EdgeElevProf: two points per edge (start-node elevation, end-node elevation).
	// EdgeElevSegDist: one distance per segment (N-1 = 1 per 2-point edge).
	const trackTags1 uint64 = 15
	g := &graph.Graph{
		NodeCount: 3, EdgeCount: 2,
		Offsets:    []uint32{0, 1, 2, 2},
		Targets:    []uint32{1, 2},
		EdgeDist:   []uint32{10000, 10000}, // 100m each
		EdgeTags1:  []uint64{trackTags1, trackTags1},
		EdgeTags2:  []uint64{0, 0},
		EdgeOSMIdx: []uint32{0, 0},
		OSMIDPool:  []int64{1},
		NodeLat:    []int32{470000000, 470900000, 471800000},
		NodeLon:    []int32{80000000, 80000000, 80000000},
		// Elevation in decimeters: 5000dm=500m, 5100dm=510m, 5200dm=520m
		NodeEle:  []int32{5000, 5100, 5200},
		NodeTags: []uint8{0, 0, 0},
		// No intermediate geometry points (≤2 geom points → EdgeGeometry returns nil)
		EdgeGeomOff: []uint32{0, 0, 0},
		EdgeGeomLat: []int32{},
		EdgeGeomLon: []int32{},
		// Full elevation profiles: 2 points per edge, offset 0→2→4
		// Edge 0: [5000, 5100] dm; Edge 1: [5100, 5200] dm
		EdgeElevOff:  []uint32{0, 2, 4},
		EdgeElevProf: []int32{5000, 5100, 5100, 5200},
		// Segment distances (N-1 per edge = 1 per edge): 10000cm each
		// C05: ElevSegDist offset = elevStart - edgeIdx
		// Edge 0: elevStart=0, edgeIdx=0 → segDistStart=0, 1 entry
		// Edge 1: elevStart=2, edgeIdx=1 → segDistStart=1, 1 entry
		// Total: 2 entries
		EdgeElevSegDist: []uint16{10000, 10000},
		IsBorder:        []bool{false, false, false},
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	reader := &SingleGraphReader{G: g, TileID: 0}
	cfg := DefaultCostConfig()

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 2)

	pass0, err := findRouteUnified(context.Background(), reader, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}
	trackIndex := buildTrackIndexUnified(pass0.state, src, dst)

	pass1, err := findRouteUnified(context.Background(), reader, src, dst, cfg, 0, pass0.totalCost, trackIndex, nil)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	result := retrackUnified(reader, pass1.state, src, dst)
	if result == nil {
		t.Fatal("retrack returned nil")
	}

	// Strictly ascending profile: AscentM > 0, DescentM == 0
	if result.AscentM <= 0 {
		t.Errorf("expected AscentM > 0 for ascending route, got %.2f", result.AscentM)
	}
	if result.DescentM != 0 {
		t.Errorf("expected DescentM == 0 for pure ascending route, got %.2f", result.DescentM)
	}
}

// TestBuildTrackIndex_RemainingCosts verifies the invariants of buildTrackIndexUnified:
// - trackIndex[src] == totalCost (maximum remaining cost at start)
// - trackIndex[dst] == 0        (no remaining cost at destination)
func TestBuildTrackIndex_RemainingCosts(t *testing.T) {
	g := makeChainGraph(4) // 4 nodes, 3 edges: 0→1→2→3
	reader := &SingleGraphReader{G: g, TileID: 0}
	cfg := DefaultCostConfig()

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 3)

	pass0, err := findRouteUnified(context.Background(), reader, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}

	trackIndex := buildTrackIndexUnified(pass0.state, src, dst)
	if trackIndex == nil {
		t.Fatal("trackIndex must not be nil")
	}

	// trackIndex[src] should equal totalCost (remaining = totalCost - 0 = totalCost)
	srcRemaining, ok := trackIndex[src]
	if !ok {
		t.Fatal("src not in trackIndex")
	}
	if srcRemaining != pass0.totalCost {
		t.Errorf("trackIndex[src]: expected %.6f (totalCost), got %.6f", pass0.totalCost, srcRemaining)
	}

	// trackIndex[dst] should equal 0 (remaining = totalCost - totalCost = 0)
	dstRemaining, ok := trackIndex[dst]
	if !ok {
		t.Fatal("dst not in trackIndex")
	}
	if dstRemaining != 0 {
		t.Errorf("trackIndex[dst]: expected 0, got %.6f", dstRemaining)
	}

	// All intermediate nodes should have remaining costs between 0 and totalCost
	for nodeID, remaining := range trackIndex {
		if remaining < 0 {
			t.Errorf("node %d has negative remaining cost: %.6f", nodeID, remaining)
		}
		if remaining > pass0.totalCost+0.001 {
			t.Errorf("node %d has remaining cost %.6f > totalCost %.6f", nodeID, remaining, pass0.totalCost)
		}
	}
}

func TestFindRouteUnified_BasicRoute(t *testing.T) {
	g := makePassableTestGraph() // 3 nodes, 2 edges, highway=track
	reader := &SingleGraphReader{G: g, TileID: 0}

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 1) // node 1 (target of edge 0)

	cfg := DefaultCostConfig()

	// Pass0
	pass0, err := findRouteUnified(context.Background(), reader, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}
	if pass0.totalCost <= 0 {
		t.Error("pass0 cost should be > 0")
	}

	trackIndex := buildTrackIndexUnified(pass0.state, src, dst)
	if trackIndex == nil {
		t.Fatal("trackIndex should not be nil")
	}

	// Pass1
	pass1, err := findRouteUnified(context.Background(), reader, src, dst, cfg, 0, pass0.totalCost, trackIndex, nil)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	// Retrack
	result := retrackUnified(reader, pass1.state, src, dst)
	if result == nil {
		t.Fatal("retrack returned nil")
	}
	if len(result.Coordinates) < 2 {
		t.Errorf("expected >= 2 coordinates, got %d", len(result.Coordinates))
	}
	if result.DistanceM <= 0 {
		t.Error("distance should be > 0")
	}
}
