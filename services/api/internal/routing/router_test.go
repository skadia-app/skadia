// router_test.go
package routing

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// makeTestGraph creates a small CSR graph for testing.
// nodes: slice of {lat, lon, ele_dm}
// edges: slice of {from, to, dist_cm, highway_enum}
func makeTestGraph(nodes [][3]float64, edges [][4]int) *graph.Graph {
	nodeCount := uint32(len(nodes))
	edgeCount := uint32(len(edges))

	g := &graph.Graph{
		NodeCount: nodeCount,
		EdgeCount: edgeCount,
		NodeLat:   make([]int32, nodeCount),
		NodeLon:   make([]int32, nodeCount),
		NodeEle:   make([]int32, nodeCount),
		NodeTags:  make([]uint8, nodeCount),
	}

	for i, n := range nodes {
		g.NodeLat[i] = int32(n[0] * 1e7)
		g.NodeLon[i] = int32(n[1] * 1e7)
		g.NodeEle[i] = int32(n[2]) // already in dm
	}

	// Count degrees
	degree := make([]uint32, nodeCount)
	for _, e := range edges {
		degree[e[0]]++
	}

	g.Offsets = make([]uint32, nodeCount+1)
	for i := range nodeCount {
		g.Offsets[i+1] = g.Offsets[i] + degree[i]
	}

	g.Targets = make([]uint32, edgeCount)
	g.EdgeDist = make([]uint32, edgeCount)
	g.EdgeTags1 = make([]uint64, edgeCount)
	g.EdgeTags2 = make([]uint64, edgeCount)
	g.EdgeOSMIdx = make([]uint32, edgeCount)
	g.OSMIDPool = []int64{12345} // single dummy OSM ID
	g.EdgeGeomOff = make([]uint32, edgeCount+1)
	g.EdgeElevOff = make([]uint32, edgeCount+1)

	cursor := make([]uint32, nodeCount)
	copy(cursor, g.Offsets[:nodeCount])
	for _, e := range edges {
		pos := cursor[e[0]]
		cursor[e[0]]++
		g.Targets[pos] = uint32(e[1])
		g.EdgeDist[pos] = uint32(e[2])
		// Encode highway enum
		g.EdgeTags1[pos] = uint64(e[3])
	}

	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	g.BBox = [4]float64{
		float64(g.NodeLat[0]) / 1e7, float64(g.NodeLon[0]) / 1e7,
		float64(g.NodeLat[0]) / 1e7, float64(g.NodeLon[0]) / 1e7,
	}
	for i := uint32(1); i < nodeCount; i++ {
		lat := float64(g.NodeLat[i]) / 1e7
		lon := float64(g.NodeLon[i]) / 1e7
		if lat < g.BBox[0] {
			g.BBox[0] = lat
		}
		if lon < g.BBox[1] {
			g.BBox[1] = lon
		}
		if lat > g.BBox[2] {
			g.BBox[2] = lat
		}
		if lon > g.BBox[3] {
			g.BBox[3] = lon
		}
	}

	return g
}

func simpleCostConfig() *CostConfig {
	cfg := &CostConfig{
		UphillCostFactor:   1.0,
		DownhillCostFactor: 0.0,
		ElevPenaltyBuffer:  5.0,
		ElevMaxBuffer:      10.0,
		Pass0Coefficient:   1.5,
	}
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCostFactor, cfg.DownhillCostFactor)
	// Simple cost: distance in meters (EdgeDist is in cm)
	cfg.EdgeCost = func(g *graph.Graph, edgeIdx uint32) float32 {
		return float32(g.EdgeDist[edgeIdx]) / 100.0
	}
	return cfg
}

func TestRoute_Triangle_ShortestPath(t *testing.T) {
	// Triangle: 0→1 (100m), 0→2 (500m), 1→2 (100m)
	// Shortest 0→2 should go via 1 (200m total)
	nodes := [][3]float64{
		{47.0, 11.0, 5000},     // node 0, 500m ele
		{47.001, 11.001, 5000}, // node 1
		{47.002, 11.0, 5000},   // node 2
	}
	edges := [][4]int{
		{0, 1, 10000, 16}, // 0→1, 100m, path
		{0, 2, 50000, 16}, // 0→2, 500m, path
		{1, 2, 10000, 16}, // 1→2, 100m, path
		{1, 0, 10000, 16}, // reverse edges
		{2, 0, 50000, 16},
		{2, 1, 10000, 16},
	}

	g := makeTestGraph(nodes, edges)
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.002, ToLon: 11.0,
	}, cfg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DistanceM > 300 {
		t.Errorf("expected route via node 1 (~200m), got %.0fm", result.DistanceM)
	}
	if len(result.Coordinates) < 2 {
		t.Errorf("expected at least 2 coordinates, got %d", len(result.Coordinates))
	}
}

func TestRoute_Diamond_HysteresisChoice(t *testing.T) {
	// Diamond: 0→1→3 (flat, long) vs 0→2→3 (short, steep climb)
	// With hysteresis cost, flat route should win
	nodes := [][3]float64{
		{47.0, 11.0, 5000},     // node 0, 500m
		{47.005, 11.005, 5000}, // node 1, 500m (flat)
		{47.005, 10.995, 7000}, // node 2, 700m (200m higher)
		{47.01, 11.0, 5000},    // node 3, 500m
	}
	edges := [][4]int{
		{0, 1, 80000, 16}, // 0→1, 800m flat
		{0, 2, 30000, 16}, // 0→2, 300m steep up
		{1, 3, 80000, 16}, // 1→3, 800m flat
		{2, 3, 30000, 16}, // 2→3, 300m steep down
		// Reverse
		{1, 0, 80000, 16},
		{2, 0, 30000, 16},
		{3, 1, 80000, 16},
		{3, 2, 30000, 16},
	}

	g := makeTestGraph(nodes, edges)
	r := NewRouter(g, 1, 5)

	// High uphill cost → prefer flat route
	cfg := simpleCostConfig()
	cfg.UphillCostFactor = 5.0
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCostFactor, cfg.DownhillCostFactor)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.01, ToLon: 11.0,
	}, cfg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With high uphill cost, should take flat route (via node 1)
	// Distance ~1600m (flat) instead of ~600m (steep)
	if result.DistanceM < 1000 {
		t.Errorf("expected flat route (>1000m), got %.0fm — hysteresis not working", result.DistanceM)
	}
}

func TestRoute_Unreachable_ReturnsError(t *testing.T) {
	// Two disconnected nodes
	nodes := [][3]float64{
		{47.0, 11.0, 5000},
		{48.0, 12.0, 5000}, // far away, no edges
	}
	edges := [][4]int{} // no edges at all

	g := makeTestGraph(nodes, edges)
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 48.0, ToLon: 12.0,
	}, cfg)

	if err == nil {
		t.Error("expected error for unreachable destination")
	}
}

func TestRoute_ContextCancelled_ReturnsEarly(t *testing.T) {
	nodes := [][3]float64{
		{47.0, 11.0, 5000},
		{47.1, 11.1, 5000},
	}
	edges := [][4]int{
		{0, 1, 100000, 16},
		{1, 0, 100000, 16},
	}

	g := makeTestGraph(nodes, edges)
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.1, ToLon: 11.1,
	}, cfg)

	if err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestRoute_ViaPoint_TwoLegs(t *testing.T) {
	nodes := [][3]float64{
		{47.0, 11.0, 5000},   // start
		{47.005, 11.0, 5000}, // via
		{47.01, 11.0, 5000},  // end
	}
	edges := [][4]int{
		{0, 1, 50000, 16},
		{1, 0, 50000, 16},
		{1, 2, 50000, 16},
		{2, 1, 50000, 16},
	}

	g := makeTestGraph(nodes, edges)
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := r.RouteVia(ctx, []LatLon{
		{47.0, 11.0},
		{47.005, 11.0},
		{47.01, 11.0},
	}, cfg)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Coordinates) < 3 {
		t.Errorf("expected at least 3 coordinates, got %d", len(result.Coordinates))
	}
}

func TestRoute_GeoJSON_Output(t *testing.T) {
	nodes := [][3]float64{
		{47.0, 11.0, 5000},
		{47.01, 11.01, 5100},
	}
	edges := [][4]int{
		{0, 1, 150000, 16}, // path
		{1, 0, 150000, 16},
	}

	g := makeTestGraph(nodes, edges)
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()

	ctx := context.Background()
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.01, ToLon: 11.01,
	}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	geoJSON := result.ToGeoJSON()
	if len(geoJSON) == 0 {
		t.Error("GeoJSON should not be empty")
	}
	// Basic sanity: should contain "FeatureCollection"
	if !strings.Contains(string(geoJSON), "FeatureCollection") {
		t.Error("GeoJSON should contain FeatureCollection")
	}
}

func TestRoute_CostCutting_GridRoute(t *testing.T) {
	// Larger grid to see cost-cutting effect
	g := makeTestGraph(
		func() [][3]float64 {
			var nodes [][3]float64
			for r := range 20 {
				for c := range 20 {
					nodes = append(nodes, [3]float64{
						47.0 + float64(r)*0.001,
						11.0 + float64(c)*0.001,
						5000,
					})
				}
			}
			return nodes
		}(),
		func() [][4]int {
			var edges [][4]int
			for r := range 20 {
				for c := range 20 {
					n := r*20 + c
					if c < 19 {
						edges = append(edges, [4]int{n, n + 1, 10000, 16})
						edges = append(edges, [4]int{n + 1, n, 10000, 16})
					}
					if r < 19 {
						edges = append(edges, [4]int{n, n + 20, 10000, 16})
						edges = append(edges, [4]int{n + 20, n, 10000, 16})
					}
				}
			}
			return edges
		}(),
	)
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()

	ctx := context.Background()
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.019, ToLon: 11.019,
	}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On a flat uniform grid, pass0's A* with heuristic is efficient.
	// Pass1's Dijkstra may expand more nodes (no heuristic), but cost-cutting
	// bounds the search. The key invariant: pass1 finds an optimal route
	// (cost ≤ pass0's upper bound). Both passes must have expanded nodes.
	if result.PassStats.Pass0Expansions == 0 {
		t.Error("pass0 should have expanded some nodes")
	}
	if result.PassStats.Pass1Expansions == 0 {
		t.Error("pass1 should have expanded some nodes")
	}
	// Route should be close to Manhattan distance on 20x20 grid (38 edges × 100m = 3800m)
	if result.DistanceM < 3500 || result.DistanceM > 4200 {
		t.Errorf("expected ~3800m route on 20x20 grid, got %.0fm", result.DistanceM)
	}
}

func TestRoute_SparseFallback_SameResult(t *testing.T) {
	// Pool size 0 forces ALL requests through sparse-map path.
	// Same triangle graph as TestRoute_Triangle_ShortestPath.
	nodes := [][3]float64{
		{47.0, 11.0, 5000},
		{47.001, 11.001, 5000},
		{47.002, 11.0, 5000},
	}
	edges := [][4]int{
		{0, 1, 10000, 16}, {0, 2, 50000, 16}, {1, 2, 10000, 16},
		{1, 0, 10000, 16}, {2, 0, 50000, 16}, {2, 1, 10000, 16},
	}
	g := makeTestGraph(nodes, edges)
	rSparse := NewRouter(g, 0, 5) // pool=0 → all sparse
	rFlat := NewRouter(g, 1, 5)   // pool=1 → flat arrays
	cfg := simpleCostConfig()

	ctx := context.Background()
	resSparse, err := rSparse.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.002, ToLon: 11.0,
	}, cfg)
	if err != nil {
		t.Fatalf("sparse: %v", err)
	}
	resFlat, err := rFlat.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.002, ToLon: 11.0,
	}, cfg)
	if err != nil {
		t.Fatalf("flat: %v", err)
	}

	if len(resSparse.Coordinates) != len(resFlat.Coordinates) {
		t.Errorf("coordinate count mismatch: sparse=%d, flat=%d",
			len(resSparse.Coordinates), len(resFlat.Coordinates))
	}
	if math.Abs(resSparse.DistanceM-resFlat.DistanceM) > 0.1 {
		t.Errorf("distance mismatch: sparse=%.1f, flat=%.1f",
			resSparse.DistanceM, resFlat.DistanceM)
	}
}
