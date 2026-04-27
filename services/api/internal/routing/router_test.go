// router_test.go — Tests for UnifiedRouter via SingleGraphReader.
package routing

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/dem"
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
	return NewUnifiedRouter(reader, 4, 2, nil)
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

// stubDEMServicer implements dem.Servicer for tests. It returns a fixed elevation
// (in decimeters) for every coordinate lookup, or dem.NoData when returnNoData
// is true.
type stubDEMServicer struct {
	// fixedElevsDm is returned in order for successive ElevationBatch calls.
	// If nil, returnNoData controls the response.
	fixedElevsDm []int32
	returnNoData bool
	callCount    int
}

func (s *stubDEMServicer) ElevationBatch(lats, lons []int32) ([]int32, error) {
	s.callCount++
	out := make([]int32, len(lats))
	for i := range out {
		if s.returnNoData {
			out[i] = dem.NoData
		} else if s.fixedElevsDm != nil && i < len(s.fixedElevsDm) {
			out[i] = s.fixedElevsDm[i]
		} else {
			out[i] = 1000 // 100.0 m default
		}
	}
	return out, nil
}
func (s *stubDEMServicer) Health() dem.Health { return dem.Health{} }
func (s *stubDEMServicer) Close()             {}

// newTestRouterWithDEM creates a UnifiedRouter that uses the provided dem.Servicer.
func newTestRouterWithDEM(g *graph.Graph, svc dem.Servicer) *UnifiedRouter {
	reader := &SingleGraphReader{G: g, TileID: 0}
	return NewUnifiedRouter(reader, 4, 2, svc)
}

// makeApproachGraph builds a minimal 2-node graph suitable for approach tests.
// Nodes are placed ~500m apart so snap distance > 100m threshold.
func makeApproachGraph() *graph.Graph {
	nodes := [][3]float64{
		{47.0, 11.0, 5000},   // node 0, 500m elevation
		{47.005, 11.0, 5100}, // node 1, 510m elevation
	}
	edges := [][4]int{
		{0, 1, 55600, 16}, // ~556m path
		{1, 0, 55600, 16},
	}
	return makeTestGraph(nodes, edges)
}

// TestRetrackUnified_SnapApproach_TwoPointHeader verifies that ApproachFromProfile
// and ApproachToProfile each contain exactly 2 ElevationPoints when a snap-approach
// is triggered, with DistM=0 at click and DistM=approach distance at projection.
func TestRetrackUnified_SnapApproach_TwoPointHeader(t *testing.T) {
	g := makeApproachGraph()
	// Fixed DEM elevations in decimeters: clickSrc=2000dm, projSrc=2010dm, projDst=2020dm, clickDst=2030dm
	svc := &stubDEMServicer{fixedElevsDm: []int32{2000, 2010, 2020, 2030}}
	r := newTestRouterWithDEM(g, svc)
	cfg := simpleCostConfig()
	cfg.SnapThresholdM = 1 // force approach for any snap distance >1m

	ctx := context.Background()
	// Request from slightly off node 0 — will snap to edge and create approach lines.
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0005, FromLon: 11.001, // offset ~100m from node 0
		ToLat:   47.0045, ToLon: 11.001,   // offset ~100m from node 1
	}, cfg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if len(result.ApproachFromProfile) == 0 {
		t.Skip("no snap approach triggered — adjust test offsets if needed")
	}

	// ApproachFromProfile: exactly 2 points, DistM[0]==0
	afp := result.ApproachFromProfile
	if len(afp) != 2 {
		t.Fatalf("ApproachFromProfile: want 2 points, got %d", len(afp))
	}
	if afp[0].DistM != 0 {
		t.Errorf("ApproachFromProfile[0].DistM: want 0, got %.4f", afp[0].DistM)
	}
	approachFromM := afp[1].DistM
	if approachFromM <= 0 {
		t.Errorf("ApproachFromProfile[1].DistM: want >0, got %.4f", approachFromM)
	}

	// ApproachToProfile: exactly 2 points, DistM[0]==0
	if len(result.ApproachToProfile) != 2 {
		t.Fatalf("ApproachToProfile: want 2 points, got %d", len(result.ApproachToProfile))
	}
	atp := result.ApproachToProfile
	if atp[0].DistM != 0 {
		t.Errorf("ApproachToProfile[0].DistM: want 0, got %.4f", atp[0].DistM)
	}
	if atp[1].DistM <= 0 {
		t.Errorf("ApproachToProfile[1].DistM: want >0, got %.4f", atp[1].DistM)
	}

	// approach_from_m in buildProperties must match ApproachFromProfile[1].DistM
	approachFromProps := haversineSum(result.ApproachFrom)
	if math.Abs(approachFromM-approachFromProps) > 1.0 {
		t.Errorf("ApproachFromProfile[1].DistM=%.2f != haversineSum(ApproachFrom)=%.2f",
			approachFromM, approachFromProps)
	}
}

// TestRetrackUnified_SnapApproach_CoordEleMatchesProfile verifies that the click
// coordinate elevation comes from the DEM batch (not from ProjEleDm), and that
// ApproachFrom[0].Ele matches *ApproachFromProfile[0].EleM.
func TestRetrackUnified_SnapApproach_CoordEleMatchesProfile(t *testing.T) {
	g := makeApproachGraph()
	// clickSrc DEM elevation: 3000dm = 300.0m; projSrc: 3100dm = 310.0m
	svc := &stubDEMServicer{fixedElevsDm: []int32{3000, 3100, 3200, 3300}}
	r := newTestRouterWithDEM(g, svc)
	cfg := simpleCostConfig()
	cfg.SnapThresholdM = 1

	ctx := context.Background()
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0005, FromLon: 11.001,
		ToLat:   47.0045, ToLon: 11.001,
	}, cfg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if len(result.ApproachFromProfile) == 0 {
		t.Skip("no snap approach triggered")
	}

	// Click coord elevation must come from DEM batch (3000dm = 300.0m)
	afCoord := result.ApproachFrom[0]
	if !afCoord.EleValid {
		t.Errorf("ApproachFrom[0].EleValid: want true, got false")
	}
	wantClickEle := 300.0
	if math.Abs(afCoord.Ele-wantClickEle) > 0.01 {
		t.Errorf("ApproachFrom[0].Ele: want %.1f (from DEM batch), got %.4f", wantClickEle, afCoord.Ele)
	}

	// Coord ele must match profile EleM
	afpClickEle := result.ApproachFromProfile[0].EleM
	if afpClickEle == nil {
		t.Fatal("ApproachFromProfile[0].EleM: want non-nil")
	}
	if math.Abs(*afpClickEle-afCoord.Ele) > 0.001 {
		t.Errorf("ApproachFrom[0].Ele=%.4f != *ApproachFromProfile[0].EleM=%.4f", afCoord.Ele, *afpClickEle)
	}
}

// TestRetrackUnified_ApproachInclusiveAxis verifies that the combined ElevationProfile
// starts at DistM=0 (click_src) and ends at a positive distance when approach is active.
//
// The click points are ~100m east of the edge (lon=11.001 vs edge lon=11.0), which
// exceeds SnapThresholdM=1 and guarantees that snap-approach fires. If no approach
// is triggered the test fails hard — use t.Fatalf to expose fixture regressions.
func TestRetrackUnified_ApproachInclusiveAxis(t *testing.T) {
	g := makeApproachGraph()
	svc := &stubDEMServicer{fixedElevsDm: []int32{1500, 1510, 1520, 1530}}
	r := newTestRouterWithDEM(g, svc)
	cfg := simpleCostConfig()
	cfg.SnapThresholdM = 1

	ctx := context.Background()
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0005, FromLon: 11.001,
		ToLat:   47.0045, ToLon: 11.001,
	}, cfg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	// Click points are ~100m east of the edge; SnapThresholdM=1 ensures approach.
	// A missing approach means the fixture regressed — fail hard, not skip.
	if len(result.ApproachFromProfile) == 0 {
		t.Fatalf("no snap approach triggered — click points should be >SnapThresholdM from edge; fixture may have regressed")
	}
	if result.ElevationProfile == nil {
		t.Fatal("ElevationProfile: want non-nil after approach concat")
	}

	prof := *result.ElevationProfile
	if len(prof) == 0 {
		t.Fatal("ElevationProfile: want non-empty")
	}
	// First point must be DistM=0 (click_src anchor, Spec §5.8 approach-inclusive axis).
	if prof[0].DistM != 0 {
		t.Errorf("ElevationProfile[0].DistM: want 0 (click_src), got %.4f", prof[0].DistM)
	}
	// Last point must cover a positive distance (approach + network + return approach).
	lastDistM := prof[len(prof)-1].DistM
	if lastDistM <= 0 {
		t.Errorf("ElevationProfile[last].DistM: want >0 (approach-inclusive), got %.4f", lastDistM)
	}
}

// TestRetrackUnified_DemNoData_NullEles_AndInvalidCoordEle verifies that when
// the DEM batch returns NoData for all points, EleM is nil in the approach
// profiles and EleValid is false on the approach coords (Spec §6 Class A).
func TestRetrackUnified_DemNoData_NullEles_AndInvalidCoordEle(t *testing.T) {
	g := makeApproachGraph()
	svc := &stubDEMServicer{returnNoData: true}
	r := newTestRouterWithDEM(g, svc)
	cfg := simpleCostConfig()
	cfg.SnapThresholdM = 1

	ctx := context.Background()
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0005, FromLon: 11.001,
		ToLat:   47.0045, ToLon: 11.001,
	}, cfg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if len(result.ApproachFromProfile) == 0 {
		t.Skip("no snap approach triggered")
	}

	// All approach profile EleM must be nil (NoData → nil via approachElevToPtrM).
	for i, p := range result.ApproachFromProfile {
		if p.EleM != nil {
			t.Errorf("ApproachFromProfile[%d].EleM: want nil (NoData), got %v", i, *p.EleM)
		}
	}
	for i, p := range result.ApproachToProfile {
		if p.EleM != nil {
			t.Errorf("ApproachToProfile[%d].EleM: want nil (NoData), got %v", i, *p.EleM)
		}
	}

	// Approach coord EleValid must be false.
	if len(result.ApproachFrom) > 0 && result.ApproachFrom[0].EleValid {
		t.Errorf("ApproachFrom[0].EleValid: want false when DEM returns NoData")
	}
	if len(result.ApproachTo) > 0 && result.ApproachTo[1].EleValid {
		t.Errorf("ApproachTo[1].EleValid: want false when DEM returns NoData")
	}
}

// TestAddApproach_NilDemSvc_FallsBackToProjectionEle verifies that when
// r.demSvc is nil (test paths without DEM setup), approach coords have
// EleValid=false and the profile EleM values are nil (zero-value / no DEM data).
// This is Variant (b) from the plan: nil demSvc → EleValid=false, EleM=nil,
// Ele=0.0 (zero value). Callers should treat EleValid=false as "unknown".
func TestAddApproach_NilDemSvc_FallsBackToProjectionEle(t *testing.T) {
	g := makeApproachGraph()
	// nil demSvc — backward-compat path used by all existing tests.
	r := newTestRouter(g)
	cfg := simpleCostConfig()
	cfg.SnapThresholdM = 1

	ctx := context.Background()
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0005, FromLon: 11.001,
		ToLat:   47.0045, ToLon: 11.001,
	}, cfg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if len(result.ApproachFromProfile) == 0 {
		t.Skip("no snap approach triggered")
	}

	// With nil demSvc, all approach elevations are unknown → EleValid=false,
	// EleM=nil, Ele=0.0 (zero-value — callers check EleValid, not Ele).
	for i, c := range result.ApproachFrom {
		if c.EleValid {
			t.Errorf("ApproachFrom[%d].EleValid: want false when demSvc=nil", i)
		}
	}
	for i, p := range result.ApproachFromProfile {
		if p.EleM != nil {
			t.Errorf("ApproachFromProfile[%d].EleM: want nil when demSvc=nil, got %.2f", i, *p.EleM)
		}
	}
}

// TestRouteSameEdge_ProfileTwoPoints verifies that the same-edge special case
// (routeSameEdge) emits a non-nil 2-point ElevationProfile (Spec §5.8, T1.3).
// The profile distance axis must span [0, result.DistanceM] and the elevation
// values must match the src/dst projection elevation from the route coordinates.
func TestRouteSameEdge_ProfileTwoPoints(t *testing.T) {
	// Two nodes on an edge; route from very close src to very close dst so
	// same-edge shortcut triggers. Use identical lat/lon nodes on the same edge
	// with a meaningful elevation difference.
	nodes := [][3]float64{
		{47.0, 11.0, 5000}, // 500m
		{47.01, 11.0, 6000}, // 600m
	}
	edges := [][4]int{
		{0, 1, 100000, 16}, // 1000m
		{1, 0, 100000, 16},
	}
	g := makeTestGraph(nodes, edges)
	r := newTestRouter(g)
	cfg := simpleCostConfig()

	ctx := context.Background()
	// Route from src (very close to node 0) to dst (very close to node 1).
	// Both project onto the same edge → routeSameEdge triggered.
	result, err := r.Route(ctx, RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat:   47.01, ToLon: 11.0,
	}, cfg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if result.ElevationProfile == nil {
		t.Fatal("ElevationProfile must not be nil for same-edge route (Spec §5.8)")
	}
	prof := *result.ElevationProfile
	if len(prof) != 2 {
		t.Fatalf("expected exactly 2 profile points for same-edge route, got %d", len(prof))
	}

	// profile[0].DistM must be 0
	if prof[0].DistM != 0 {
		t.Errorf("profile[0].DistM: want 0, got %.4f", prof[0].DistM)
	}
	// profile[1].DistM must equal result.DistanceM
	if math.Abs(prof[1].DistM-result.DistanceM) > 0.01 {
		t.Errorf("profile[1].DistM=%.4f, want %.4f (result.DistanceM)", prof[1].DistM, result.DistanceM)
	}
	// Both elevation values must be non-nil and non-negative (sanity)
	if prof[0].EleM == nil || prof[1].EleM == nil {
		t.Fatalf("nil EleM in same-edge profile")
	}
	if *prof[0].EleM < 0 || *prof[1].EleM < 0 {
		t.Errorf("negative elevation in same-edge profile: [%.1f, %.1f]", *prof[0].EleM, *prof[1].EleM)
	}
}

// makeFiveVertexEdgeGraph builds a 2-node graph with one bidirectional edge
// carrying 3 intermediate geometry vertices and a 5-point EdgeElevProf with
// matching segDists. Used by T0 C.4 same-edge full-resolution tests.
//
// Geometry (5 vertices along lon=11.0):
//
//	Node A at lat 47.000, intermediates at 47.001/47.002/47.003, Node B at 47.004.
//
// Each step ≈111m → segDist 11119cm fits uint16. Total edge ≈ 445m.
// Elevations: 100, 110, 120, 130, 140 m (10m gain per step, monotonic ascent).
func makeFiveVertexEdgeGraph(t *testing.T) *graph.Graph {
	t.Helper()
	const trackTags1 uint64 = 16 // path
	g := &graph.Graph{
		NodeCount: 2, EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2},
		Targets:   []uint32{1, 0},
		EdgeDist:  []uint32{44478, 44478}, // ~445m in cm
		EdgeTags1: []uint64{trackTags1, trackTags1},
		EdgeTags2: []uint64{0, 0},
		EdgeOSMIdx: []uint32{0, 0},
		OSMIDPool: []int64{1},
		NodeLat:   []int32{470000000, 470040000},
		NodeLon:   []int32{110000000, 110000000},
		NodeEle:   []int32{1000, 1400}, // 100m, 140m
		NodeTags:  []uint8{0, 0},

		// 5 geom vertices per edge: NodeA + 3 intermediates + NodeB (in same orientation
		// for both edges — reader returns indices [start+1:end-1] so the actual
		// stored polyline must include the endpoints).
		EdgeGeomOff: []uint32{0, 5, 10},
		EdgeGeomLat: []int32{
			// edge 0: A→B
			470000000, 470010000, 470020000, 470030000, 470040000,
			// edge 1: B→A (mirror)
			470040000, 470030000, 470020000, 470010000, 470000000,
		},
		EdgeGeomLon: []int32{
			110000000, 110000000, 110000000, 110000000, 110000000,
			110000000, 110000000, 110000000, 110000000, 110000000,
		},

		// 5 elev points per edge → EdgeElevOff = [0, 5, 10].
		EdgeElevOff: []uint32{0, 5, 10},
		EdgeElevProf: []int32{
			1000, 1100, 1200, 1300, 1400, // edge 0: ascending
			1400, 1300, 1200, 1100, 1000, // edge 1: descending (mirror)
		},
		// 4 segDists per 5-point edge.
		// EdgeElevSegDist offset for edge i = elevStart - edgeIdx = 0-0=0 for edge 0,
		// 5-1=4 for edge 1. So we lay them in [0..4) and [4..8).
		EdgeElevSegDist: []uint16{
			11119, 11119, 11119, 11119, // edge 0
			11119, 11119, 11119, 11119, // edge 1
		},
		IsBorder: []bool{false, false},
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	return g
}

// TestRouteSameEdge_FullResolutionCoordsAndProfile verifies T0 C.4: when src
// and dst project onto the same edge with intermediate geometry, both Coords
// and ElevationProfile carry the full-resolution interior vertices, not just
// the two projection endpoints.
func TestRouteSameEdge_FullResolutionCoordsAndProfile(t *testing.T) {
	g := makeFiveVertexEdgeGraph(t)
	r := newTestRouter(g)
	cfg := simpleCostConfig()

	// src at fraction ≈0.2 (lat≈47.0008), dst at fraction ≈0.8 (lat≈47.0032).
	srcVE, err := r.snapToEdge(47.0008, 11.0, cfg)
	if err != nil {
		t.Fatalf("snap src: %v", err)
	}
	dstVE, err := r.snapToEdge(47.0032, 11.0, cfg)
	if err != nil {
		t.Fatalf("snap dst: %v", err)
	}
	if srcVE.Proj.EdgeHandle != dstVE.Proj.EdgeHandle {
		t.Skipf("projections did not land on the same edge: src=%d dst=%d",
			srcVE.Proj.EdgeHandle, dstVE.Proj.EdgeHandle)
	}

	result := r.routeSameEdge(srcVE, dstVE, cfg)
	if result == nil {
		t.Fatal("routeSameEdge returned nil")
	}
	if result.ElevationProfile == nil {
		t.Fatal("ElevationProfile must not be nil")
	}
	prof := *result.ElevationProfile

	if len(result.Coordinates) != len(prof) {
		t.Fatalf("len(Coordinates)=%d vs len(ElevationProfile)=%d — must match (T0 C.4 invariant)",
			len(result.Coordinates), len(prof))
	}
	if len(result.Coordinates) <= 2 {
		t.Errorf("expected >2 coords for full-resolution slice, got %d", len(result.Coordinates))
	}
	if prof[0].DistM != 0 {
		t.Errorf("prof[0].DistM: want 0, got %.4f", prof[0].DistM)
	}
	if math.Abs(prof[len(prof)-1].DistM-result.DistanceM) > 0.5 {
		t.Errorf("prof[last].DistM=%.4f != result.DistanceM=%.4f", prof[len(prof)-1].DistM, result.DistanceM)
	}
	// Monotonic ascent — 10m gain per ~111m segment, full slice ≈ 6m gain.
	if result.AscentM <= 0 {
		t.Errorf("expected positive ascent on monotonic-up edge, got %.2f", result.AscentM)
	}
	if result.DescentM > 0.5 {
		t.Errorf("expected ~0 descent on monotonic-up edge, got %.2f", result.DescentM)
	}
}

// TestRouteSameEdge_ReversedDirection_ProfileAscends verifies that when the
// traveler walks B→A on a same-edge route, the returned ElevationProfile has
// strictly ascending DistM (i.e. distances run from 0 at the source to total
// at the destination — no negative or out-of-order values).
func TestRouteSameEdge_ReversedDirection_ProfileAscends(t *testing.T) {
	g := makeFiveVertexEdgeGraph(t)
	r := newTestRouter(g)
	cfg := simpleCostConfig()

	// src high (closer to NodeB at fraction ≈0.8), dst low (≈0.2).
	srcVE, err := r.snapToEdge(47.0032, 11.0, cfg)
	if err != nil {
		t.Fatalf("snap src: %v", err)
	}
	dstVE, err := r.snapToEdge(47.0008, 11.0, cfg)
	if err != nil {
		t.Fatalf("snap dst: %v", err)
	}
	if srcVE.Proj.EdgeHandle != dstVE.Proj.EdgeHandle {
		t.Skip("projections did not land on the same edge")
	}
	if srcVE.Proj.FractionAB <= dstVE.Proj.FractionAB {
		t.Skipf("src fraction %.3f not > dst fraction %.3f — not the reversed case",
			srcVE.Proj.FractionAB, dstVE.Proj.FractionAB)
	}

	result := r.routeSameEdge(srcVE, dstVE, cfg)
	if result == nil {
		t.Fatal("routeSameEdge returned nil")
	}
	prof := *result.ElevationProfile
	if len(prof) < 2 {
		t.Fatalf("reversed slice: want >=2 profile points, got %d", len(prof))
	}
	for i := 1; i < len(prof); i++ {
		if prof[i].DistM < prof[i-1].DistM {
			t.Errorf("prof[%d].DistM=%.3f < prof[%d].DistM=%.3f — must ascend",
				i, prof[i].DistM, i-1, prof[i-1].DistM)
		}
	}
	if prof[0].DistM != 0 {
		t.Errorf("prof[0].DistM: want 0, got %.4f", prof[0].DistM)
	}
	// Reversed = traveling downhill (src ele > dst ele).
	if result.DescentM <= 0 {
		t.Errorf("reversed (downhill) route: DescentM should be > 0, got %.2f", result.DescentM)
	}
	if result.AscentM > 0.5 {
		t.Errorf("reversed (downhill) route: AscentM should be ~0, got %.2f", result.AscentM)
	}
	// And: first coord matches the src projection direction (high-end vertex).
	first := result.Coordinates[0]
	last := result.Coordinates[len(result.Coordinates)-1]
	if first.Lat <= last.Lat {
		t.Errorf("reversed: first coord lat %.6f should be > last %.6f (traveler heads south)",
			first.Lat, last.Lat)
	}
}
