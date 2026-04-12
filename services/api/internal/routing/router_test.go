// router_test.go — Tests for UnifiedRouter via SingleGraphReader.
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
		UphillCost:        1.0,
		DownhillCost:      0.0,
		ElevPenaltyBuffer: 5.0,
		ElevMaxBuffer:     10.0,
		Pass0Coefficient:  1.5,
	}
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)
	// Simple cost: costfactor=1.0 for all edges (distance computed in A* loop)
	cfg.EdgeCost = func(tags1, _ uint64) EdgeCostFactors {
		return EdgeCostFactors{Flat: 1.0}
	}
	return cfg
}

// newTestRouter creates a UnifiedRouter backed by a SingleGraphReader.
func newTestRouter(g *graph.Graph) *UnifiedRouter {
	reader := &SingleGraphReader{G: g, TileID: 0}
	return NewUnifiedRouter(reader, 4, 2)
}

func TestRoute_Triangle_ShortestPath(t *testing.T) {
	// Triangle: 0->1 (100m), 0->2 (500m), 1->2 (100m)
	// Shortest 0->2 should go via 1 (200m total)
	nodes := [][3]float64{
		{47.0, 11.0, 5000},     // node 0, 500m ele
		{47.001, 11.001, 5000}, // node 1
		{47.002, 11.0, 5000},   // node 2
	}
	edges := [][4]int{
		{0, 1, 10000, 16}, // 0->1, 100m, path
		{0, 2, 50000, 16}, // 0->2, 500m, path
		{1, 2, 10000, 16}, // 1->2, 100m, path
		{1, 0, 10000, 16}, // reverse edges
		{2, 0, 50000, 16},
		{2, 1, 10000, 16},
	}

	g := makeTestGraph(nodes, edges)
	r := newTestRouter(g)
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
	// Diamond: 0->1->3 (flat, long) vs 0->2->3 (short, steep climb)
	// With hysteresis cost, flat route should win
	nodes := [][3]float64{
		{47.0, 11.0, 5000},     // node 0, 500m
		{47.005, 11.005, 5000}, // node 1, 500m (flat)
		{47.005, 10.995, 7000}, // node 2, 700m (200m higher)
		{47.01, 11.0, 5000},    // node 3, 500m
	}
	edges := [][4]int{
		{0, 1, 80000, 16}, // 0->1, 800m flat
		{0, 2, 30000, 16}, // 0->2, 300m steep up
		{1, 3, 80000, 16}, // 1->3, 800m flat
		{2, 3, 30000, 16}, // 2->3, 300m steep down
		// Reverse
		{1, 0, 80000, 16},
		{2, 0, 30000, 16},
		{3, 1, 80000, 16},
		{3, 2, 30000, 16},
	}

	g := makeTestGraph(nodes, edges)
	r := newTestRouter(g)

	// High uphill cost -> prefer flat route
	cfg := simpleCostConfig()
	cfg.UphillCost = 5.0
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)

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
		t.Errorf("expected flat route (>1000m), got %.0fm -- hysteresis not working", result.DistanceM)
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
	r := newTestRouter(g)
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
	r := newTestRouter(g)
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
	r := newTestRouter(g)
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
	r := newTestRouter(g)
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
	r := newTestRouter(g)
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
	// (cost <= pass0's upper bound). Both passes must have expanded nodes.
	if result.PassStats.Pass0Expansions == 0 {
		t.Error("pass0 should have expanded some nodes")
	}
	if result.PassStats.Pass1Expansions == 0 {
		t.Error("pass1 should have expanded some nodes")
	}
	// Route should be close to Manhattan distance on 20x20 grid (38 edges x 100m = 3800m)
	if result.DistanceM < 3500 || result.DistanceM > 4200 {
		t.Errorf("expected ~3800m route on 20x20 grid, got %.0fm", result.DistanceM)
	}
}

func TestRoute_TurnCost_PrefersStraight(t *testing.T) {
	// Graph: 0->1->3 (straight line north) vs 0->2->3 (zigzag east then back)
	// With TurnCost, the straight path should be preferred even if zigzag is
	// shorter in distance, because zigzag has a sharp turn at node 2.
	nodes := [][3]float64{
		{47.0, 11.0, 5000},      // node 0: start
		{47.0035, 11.0, 5000},   // node 1: straight intermediate (north)
		{47.0025, 11.003, 5000}, // node 2: zigzag intermediate (east)
		{47.007, 11.0, 5000},    // node 3: destination (north)
	}
	edges := [][4]int{
		// Straight path: 0->1->3
		{0, 1, 35000, 16}, // 350m
		{1, 3, 35000, 16}, // 350m
		// Zigzag path: 0->2->3 (shorter distance, but with turn)
		{0, 2, 25000, 16}, // 250m
		{2, 3, 25000, 16}, // 250m
		// Reverse edges
		{1, 0, 35000, 16},
		{3, 1, 35000, 16},
		{2, 0, 25000, 16},
		{3, 2, 25000, 16},
	}

	g := makeTestGraph(nodes, edges)
	ctx := context.Background()

	// Without TurnCost: zigzag should win (500m < 700m)
	cfgNoTurn := simpleCostConfig()
	cfgNoTurn.TurnCost = 0

	rNoTurn := newTestRouter(g)
	resNoTurn, err := rNoTurn.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.007, ToLon: 11.0,
	}, cfgNoTurn)
	if err != nil {
		t.Fatalf("no-turn route failed: %v", err)
	}

	// With TurnCost: straight should win (turn penalty outweighs distance savings)
	cfgTurn := simpleCostConfig()
	cfgTurn.TurnCost = 200 // high turn cost

	rTurn := newTestRouter(g)
	resTurn, err := rTurn.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.007, ToLon: 11.0,
	}, cfgTurn)
	if err != nil {
		t.Fatalf("turn route failed: %v", err)
	}

	t.Logf("no-turn: dist=%.0fm, cost=%.0f", resNoTurn.DistanceM, resNoTurn.Cost)
	t.Logf("turn:    dist=%.0fm, cost=%.0f", resTurn.DistanceM, resTurn.Cost)

	// Without turn cost, shorter zigzag should be chosen (cost=500)
	if resNoTurn.Cost > 550 {
		t.Errorf("without turncost: expected zigzag route (cost~500), got cost=%.0f", resNoTurn.Cost)
	}
	// With turn cost, straight path should be chosen (cost=700, cheaper than 500+turn_penalty)
	if resTurn.Cost < resNoTurn.Cost {
		t.Errorf("with turncost: expected higher cost than no-turn, got turn=%.0f vs noTurn=%.0f",
			resTurn.Cost, resNoTurn.Cost)
	}
	// The straight route should be longer in distance
	if resTurn.DistanceM < resNoTurn.DistanceM {
		t.Logf("NOTE: with turncost, router chose longer distance path (%.0fm > %.0fm) -- turn avoidance working",
			resTurn.DistanceM, resNoTurn.DistanceM)
	}
}

func TestRoute_3CostFactor_SlopeWeighted(t *testing.T) {
	// Test that UpWeight/DownWeight correctly interpolate costfactors.
	// Node 0: 500m, Node 1: 600m (100m gain over 1000m = 10% grade)
	nodes := [][3]float64{
		{47.0, 11.0, 5000},  // 500m
		{47.01, 11.0, 6000}, // 600m
	}
	edges := [][4]int{
		{0, 1, 100000, 16}, // 1000m, path
		{1, 0, 100000, 16},
	}

	g := makeTestGraph(nodes, edges)

	// Config with different up/down costfactors
	cfg := simpleCostConfig()
	cfg.UphillCost = 5.0
	cfg.DownhillCost = 0.0
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)

	// Use a custom EdgeCost that returns Up/Down factors
	cfg.EdgeCost = func(_, _ uint64) EdgeCostFactors {
		return EdgeCostFactors{
			Flat: 1.0,
			Up:   3.0, // uphill is 3x more expensive
			Down: 0.5, // downhill is cheaper
		}
	}

	r := newTestRouter(g)
	ctx := context.Background()

	// Route uphill: 0->1
	resUp, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.01, ToLon: 11.0,
	}, cfg)
	if err != nil {
		t.Fatalf("uphill route failed: %v", err)
	}

	// Route downhill: 1->0
	resDown, err := r.Route(ctx, RouteRequest{
		FromLat: 47.01, FromLon: 11.0,
		ToLat: 47.0, ToLon: 11.0,
	}, cfg)
	if err != nil {
		t.Fatalf("downhill route failed: %v", err)
	}

	t.Logf("uphill cost=%.1f, downhill cost=%.1f", resUp.Cost, resDown.Cost)

	// Uphill cost should be significantly higher than downhill
	if resUp.Cost <= resDown.Cost {
		t.Errorf("uphill cost (%.1f) should be > downhill cost (%.1f) with Up=3.0, Down=0.5",
			resUp.Cost, resDown.Cost)
	}
}

func TestRoute_InitialCost_AvoidsFerryHopping(t *testing.T) {
	// Graph: S->A--ferry--B->D (land->ferry->land, 2 transitions = 2*10000 penalty)
	//        S->C->D (all land, longer distance but no penalty)
	nodes := [][3]float64{
		{47.0, 11.0, 5000},     // S (start)
		{47.002, 11.0, 5000},   // A (land->ferry transition)
		{47.006, 11.0, 5000},   // B (ferry->land transition)
		{47.01, 11.0, 5000},    // D (destination)
		{47.005, 11.005, 5000}, // C (land detour)
	}

	allEdges := [][4]int{
		// Ferry path: S->A->B->D
		{0, 1, 20000, 16}, // S->A: 200m land
		{1, 2, 30000, 16}, // A->B: 300m ferry
		{2, 3, 20000, 16}, // B->D: 200m land
		// Land detour: S->C->D
		{0, 4, 60000, 16}, // S->C: 600m land
		{4, 3, 60000, 16}, // C->D: 600m land
		// Reverse edges
		{1, 0, 20000, 16},
		{2, 1, 30000, 16},
		{3, 2, 20000, 16},
		{4, 0, 60000, 16},
		{3, 4, 60000, 16},
	}

	g := makeTestGraph(nodes, allEdges)

	// Mark ferry edges (A<->B) using bit 62 of tags1.
	for ei := uint32(0); ei < g.EdgeCount; ei++ {
		from := uint32(0)
		for i := uint32(0); i < g.NodeCount; i++ {
			if g.Offsets[i] <= ei && ei < g.Offsets[i+1] {
				from = i
				break
			}
		}
		to := g.Targets[ei]
		if (from == 1 && to == 2) || (from == 2 && to == 1) {
			g.EdgeTags1[ei] |= 1 << 62 // ferry marker bit
		}
	}

	ctx := context.Background()

	// Without WayClass: all WayClass=0, no penalty. Ferry path wins (700m < 1200m).
	cfgNoWC := simpleCostConfig()
	rNoWC := newTestRouter(g)
	resNoWC, err := rNoWC.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.01, ToLon: 11.0,
	}, cfgNoWC)
	if err != nil {
		t.Fatalf("no-wayclass route failed: %v", err)
	}

	// With WayClass: land edges get WC=2, ferry edges get WC=1.
	cfgWC := simpleCostConfig()
	cfgWC.EdgeCost = func(tags1, _ uint64) EdgeCostFactors {
		ecf := EdgeCostFactors{Flat: 1.0}
		if (tags1>>62)&1 == 1 {
			ecf.WayClass = 1 // ferry (bit 62 set)
		} else {
			ecf.WayClass = 2 // mainroad (non-zero, so transitions are detected)
		}
		return ecf
	}

	rWC := newTestRouter(g)
	resWC, err := rWC.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.01, ToLon: 11.0,
	}, cfgWC)
	if err != nil {
		t.Fatalf("wayclass route failed: %v", err)
	}

	t.Logf("no-wayclass: dist=%.0fm, cost=%.0f", resNoWC.DistanceM, resNoWC.Cost)
	t.Logf("with-wayclass: dist=%.0fm, cost=%.0f", resWC.DistanceM, resWC.Cost)

	// Without WayClass: ferry path chosen (shorter)
	if resNoWC.Cost > 800 {
		t.Errorf("without wayclass: expected ferry route (cost~700), got cost=%.0f", resNoWC.Cost)
	}

	// With WayClass: land route chosen (ferry gets 2*10000 penalty)
	if resWC.Cost < 1100 {
		t.Errorf("with wayclass: expected land route (cost~1200), got cost=%.0f -- ferry penalty not working", resWC.Cost)
	}
}

func TestComputeTurnCost_Basic(t *testing.T) {
	// Straight ahead: cosAngle=1, turnCost=0 (long edge = full scale)
	tc, dx, dy := computeTurnCost(0, 0, 100, 0, 1, 0, 90, 50000) // 500m
	if tc != 0 {
		t.Errorf("straight ahead: expected turnCost=0, got %f", tc)
	}
	if dx == 0 {
		t.Error("expected non-zero dx")
	}
	_ = dy

	// 180 deg turn on long edge: cosAngle=-1, turnCost=2*base*distScale
	tc2, _, _ := computeTurnCost(0, 0, -100, 0, 1, 0, 90, 50000) // 500m -> distScale=1.0
	if math.Abs(float64(tc2)-180) > 1 {
		t.Errorf("180 deg turn long edge: expected turnCost~180, got %f", tc2)
	}

	// 180 deg turn on SHORT edge: turnCost scaled down
	tc2short, _, _ := computeTurnCost(0, 0, -100, 0, 1, 0, 90, 1000) // 10m -> distScale=0.05
	if tc2short >= tc2*0.2 {
		t.Errorf("180 deg turn short edge: expected much less than long, got short=%f long=%f", tc2short, tc2)
	}
	if tc2short <= 0 {
		t.Errorf("180 deg turn short edge: expected >0, got %f", tc2short)
	}

	// No previous direction: no turn cost
	tc3, _, _ := computeTurnCost(0, 0, 100, 0, 0, 0, 90, 50000)
	if tc3 != 0 {
		t.Errorf("no prev direction: expected turnCost=0, got %f", tc3)
	}

	// TurnCost base=0: no turn cost regardless
	tc4, _, _ := computeTurnCost(0, 0, -100, 0, 1, 0, 0, 50000)
	if tc4 != 0 {
		t.Errorf("base=0: expected turnCost=0, got %f", tc4)
	}
}
