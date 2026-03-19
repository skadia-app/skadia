// router.go
package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// UnifiedRouter is a Router backed by any GraphReader (e.g. TiledGraphReader).
// Uses a semaphore for concurrency control instead of a pre-allocated pool.
// Coexists with the legacy Router/NewRouter (which is Graph-specific).
type UnifiedRouter struct {
	reader GraphReader
	sem    chan struct{}
}

// NewUnifiedRouter creates a router backed by any GraphReader.
// maxConcurrent limits simultaneous routing queries (semaphore-based).
func NewUnifiedRouter(reader GraphReader, maxConcurrent int) *UnifiedRouter {
	return &UnifiedRouter{
		reader: reader,
		sem:    make(chan struct{}, maxConcurrent),
	}
}

// Route computes a route from start to destination using findRouteUnified
// (pass0 → buildTrackIndexUnified → pass1 → retrackUnified).
func (r *UnifiedRouter) Route(ctx context.Context, req RouteRequest,
	cfg *CostConfig) (*RouteResult, error) {

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	totalStart := time.Now()

	src, _, ok := r.reader.NearestNode(req.FromLat, req.FromLon)
	if !ok {
		return nil, fmt.Errorf("no graph node within 5km of start coordinate")
	}
	dst, _, ok := r.reader.NearestNode(req.ToLat, req.ToLon)
	if !ok {
		return nil, fmt.Errorf("no graph node within 5km of destination coordinate")
	}

	if src == dst {
		lat, lon, ele := r.reader.NodeCoord(src)
		return &RouteResult{
			Coordinates: []LatLonEle{{
				Lat: float64(lat) / 1e7,
				Lon: float64(lon) / 1e7,
				Ele: float64(ele) / 10.0,
			}},
		}, nil
	}

	// Pass 0: inadmissible A* (passCoeff > 1)
	res0, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
		cfg.Pass0Coefficient, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("pass0: %w", err)
	}

	// Build track index from pass0 path for pass1 cost-cutting
	trackIndex := buildTrackIndexUnified(res0.state, src, dst)

	// Pass 1: Dijkstra with track-index cost-cutting (passCoeff = 0)
	res1, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
		0, res0.totalCost, trackIndex)
	if err != nil {
		return nil, fmt.Errorf("pass1: %w", err)
	}

	// Retrack: reconstruct path with full geometry
	result := retrackUnified(r.reader, res1.state, src, dst)
	if result == nil {
		return nil, fmt.Errorf("retrack failed: broken parent chain")
	}
	result.PassStats = PassStats{
		Pass0Expansions: res0.expansions,
		Pass0DurationMs: res0.durationMs,
		Pass1Expansions: res1.expansions,
		Pass1DurationMs: res1.durationMs,
		TotalDurationMs: time.Since(totalStart).Milliseconds(),
	}
	return result, nil
}

// RouteVia computes a route through multiple waypoints.
// Hysteresis state is NOT transferred between legs (reset at via-points, like BRouter).
func (r *UnifiedRouter) RouteVia(ctx context.Context, waypoints []LatLon,
	cfg *CostConfig) (*RouteResult, error) {

	if len(waypoints) < 2 {
		return nil, fmt.Errorf("need at least 2 waypoints")
	}

	var legs []*RouteResult
	for i := range len(waypoints) - 1 {
		leg, err := r.Route(ctx, RouteRequest{
			FromLat: waypoints[i].Lat, FromLon: waypoints[i].Lon,
			ToLat: waypoints[i+1].Lat, ToLon: waypoints[i+1].Lon,
		}, cfg)
		if err != nil {
			return nil, fmt.Errorf("leg %d: %w", i, err)
		}
		legs = append(legs, leg)
	}
	return mergeLegs(legs), nil
}

// RouterState holds pre-allocated working arrays for a single routing query.
// 480 MB per state at 30M nodes.
type RouterState struct {
	bestCost   []float32
	parentNode []uint32
	parentEdge []uint32
	nodeGen    []uint32
	generation uint32

	// Per-node state maps for hysteresis and last-edge direction.
	// Populated during A* expansion; read when popping (replaces FrontierEntry fields
	// removed in Task 8). Task 11 (unified A*) will replace this with the NodeState struct.
	nodeHyState map[uint32]PathState
	nodeLastEdge map[uint32]crossLastEdge

	frontier Frontier
}

func newRouterState(nodeCount uint32) *RouterState {
	return &RouterState{
		bestCost:     make([]float32, nodeCount),
		parentNode:   make([]uint32, nodeCount),
		parentEdge:   make([]uint32, nodeCount),
		nodeGen:      make([]uint32, nodeCount),
		generation:   1, // start at 1, 0 is the "unvisited" sentinel
		nodeHyState:  make(map[uint32]PathState, 500_000),
		nodeLastEdge: make(map[uint32]crossLastEdge, 500_000),
	}
}

// advanceGeneration increments the generation counter with overflow protection.
func (s *RouterState) advanceGeneration() {
	s.generation++
	if s.generation == 0 {
		s.generation = 1
		for i := range s.nodeGen {
			s.nodeGen[i] = 0
		}
	}
	clear(s.nodeHyState)
	clear(s.nodeLastEdge)
}

// sparseOnlyThreshold: graphs with more nodes than this use sparse-only
// routing (no pre-allocated flat arrays). Saves ~480 MB per hot-pool state.
const sparseOnlyThreshold uint32 = 10_000_000

// Router holds the immutable graph and a pool of RouterStates for concurrent queries.
type Router struct {
	Graph     *graph.Graph
	pool      chan *RouterState
	sparseSem chan struct{}
}

// NewRouter creates a Router with a bounded pool of pre-allocated RouterStates.
// poolSize: number of hot-path states (default 3, each ~480 MB at 30M nodes).
// maxSparse: max concurrent sparse-map queries (default 10).
func NewRouter(g *graph.Graph, poolSize int, maxSparse int) *Router {
	r := &Router{
		Graph:     g,
		pool:      make(chan *RouterState, poolSize),
		sparseSem: make(chan struct{}, maxSparse),
	}
	if g.NodeCount <= sparseOnlyThreshold {
		for range poolSize {
			r.pool <- newRouterState(g.NodeCount)
		}
	}
	// When NodeCount > threshold: pool stays empty → all queries use sparse fallback
	return r
}

// Route computes a route from start to destination.
// Uses hot-pool RouterState if available, sparse-map fallback otherwise.
func (r *Router) Route(ctx context.Context, req RouteRequest,
	cfg *CostConfig) (*RouteResult, error) {

	select {
	case state := <-r.pool:
		defer func() { r.pool <- state }()
		state.advanceGeneration()
		return r.findRoute(ctx, state, req, cfg)
	default:
		// Pool exhausted: sparse fallback with concurrency limit
		select {
		case r.sparseSem <- struct{}{}:
			defer func() { <-r.sparseSem }()
			return r.findRouteSparse(ctx, req, cfg)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// RouteVia computes a route through multiple waypoints.
// Hysteresis state is NOT transferred between legs (reset at via-points, like BRouter).
func (r *Router) RouteVia(ctx context.Context, waypoints []LatLon,
	cfg *CostConfig) (*RouteResult, error) {

	if len(waypoints) < 2 {
		return nil, fmt.Errorf("need at least 2 waypoints")
	}

	var legs []*RouteResult
	for i := 0; i < len(waypoints)-1; i++ {
		leg, err := r.Route(ctx, RouteRequest{
			FromLat: waypoints[i].Lat, FromLon: waypoints[i].Lon,
			ToLat: waypoints[i+1].Lat, ToLon: waypoints[i+1].Lon,
		}, cfg)
		if err != nil {
			return nil, fmt.Errorf("leg %d: %w", i, err)
		}
		legs = append(legs, leg)
	}
	return mergeLegs(legs), nil
}

// findRoute runs the 3-pass algorithm using flat-array RouterState.
func (r *Router) findRoute(ctx context.Context, state *RouterState,
	req RouteRequest, cfg *CostConfig) (*RouteResult, error) {

	totalStart := time.Now()
	g := r.Graph

	src, srcDist := graph.NearestNode(g, req.FromLat, req.FromLon)
	dst, dstDist := graph.NearestNode(g, req.ToLat, req.ToLon)

	// uint16: max ~6.5km; 50000 dm = 5km. NearestNode returns uint16 distance.
	const maxSnapDist uint16 = 50000
	if srcDist >= maxSnapDist {
		return nil, fmt.Errorf("no graph node within 5km of start coordinate")
	}
	if dstDist >= maxSnapDist {
		return nil, fmt.Errorf("no graph node within 5km of destination coordinate")
	}

	if src == dst {
		return &RouteResult{
			Coordinates: []LatLonEle{{
				Lat: float64(g.NodeLat[src]) / 1e7,
				Lon: float64(g.NodeLon[src]) / 1e7,
				Ele: float64(g.NodeEle[src]) / 10.0,
			}},
		}, nil
	}

	// Pass 0
	track0, maxCost, stats0, err := pass0(ctx, g, state, src, dst, cfg)
	if err != nil {
		return nil, fmt.Errorf("pass0: %w", err)
	}

	// buildTrackIndex MUST run before advanceGeneration: it reads pass0's bestCost
	// via the current generation's nodeGen entries.
	trackIndex := buildTrackIndex(state, track0)

	// Generation reset between passes
	state.advanceGeneration()

	// Pass 1 (receives pre-built trackIndex)
	stats1, err := pass1(ctx, g, state, src, dst, cfg, trackIndex, maxCost)
	if err != nil {
		return nil, fmt.Errorf("pass1: %w", err)
	}

	// Re-tracking
	result := retrack(g, state, src, dst, float64(state.bestCost[dst]))
	result.PassStats = PassStats{
		Pass0Expansions: stats0.Pass0Expansions,
		Pass0DurationMs: stats0.Pass0DurationMs,
		Pass1Expansions: stats1.Pass1Expansions,
		Pass1DurationMs: stats1.Pass1DurationMs,
		TotalDurationMs: time.Since(totalStart).Milliseconds(),
	}
	return result, nil
}

// findRouteSparse runs the 3-pass algorithm using map-based SparseState.
func (r *Router) findRouteSparse(ctx context.Context,
	req RouteRequest, cfg *CostConfig) (*RouteResult, error) {

	totalStart := time.Now()
	g := r.Graph
	state := newSparseState()

	src, srcDist := graph.NearestNode(g, req.FromLat, req.FromLon)
	dst, dstDist := graph.NearestNode(g, req.ToLat, req.ToLon)

	// uint16: max ~6.5km; 50000 dm = 5km. NearestNode returns uint16 distance.
	const maxSnapDist uint16 = 50000
	if srcDist >= maxSnapDist {
		return nil, fmt.Errorf("no graph node within 5km of start coordinate")
	}
	if dstDist >= maxSnapDist {
		return nil, fmt.Errorf("no graph node within 5km of destination coordinate")
	}

	if src == dst {
		return &RouteResult{
			Coordinates: []LatLonEle{{
				Lat: float64(g.NodeLat[src]) / 1e7,
				Lon: float64(g.NodeLon[src]) / 1e7,
				Ele: float64(g.NodeEle[src]) / 10.0,
			}},
		}, nil
	}

	// Pass 0
	track0, maxCost, stats0, err := pass0Sparse(ctx, g, state, src, dst, cfg)
	if err != nil {
		return nil, fmt.Errorf("pass0: %w", err)
	}

	// buildTrackIndexSparse MUST run before clearMaps: it reads pass0's bestCost.
	trackIndex := buildTrackIndexSparse(state, track0)

	// Clear maps for pass 1 reuse
	state.clearMaps()

	// Pass 1 (receives pre-built trackIndex)
	stats1, err := pass1Sparse(ctx, g, state, src, dst, cfg, trackIndex, maxCost)
	if err != nil {
		return nil, fmt.Errorf("pass1: %w", err)
	}

	// Re-tracking
	result := retrackSparse(g, state, src, dst, float64(state.bestCost[dst]))
	result.PassStats = PassStats{
		Pass0Expansions: stats0.Pass0Expansions,
		Pass0DurationMs: stats0.Pass0DurationMs,
		Pass1Expansions: stats1.Pass1Expansions,
		Pass1DurationMs: stats1.Pass1DurationMs,
		TotalDurationMs: time.Since(totalStart).Milliseconds(),
	}
	return result, nil
}
