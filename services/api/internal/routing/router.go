// router.go — UnifiedRouter backed by any GraphReader.
package routing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// altDisabled disables the ALT heuristic when SKADIA_ALT_DISABLED=true.
// Useful for A/B comparisons and debugging.
var altDisabled = os.Getenv("SKADIA_ALT_DISABLED") == "true"

var routingBidirectional = os.Getenv("ROUTING_BIDIRECTIONAL") == "true"
var routingValleSeg = os.Getenv("ROUTING_VALLE_SEG") == "true"

// UnifiedRouter is a Router backed by any GraphReader (e.g. TiledGraphReader).
// Uses a semaphore for concurrency control instead of a pre-allocated pool.
type UnifiedRouter struct {
	reader GraphReader
	sem    chan struct{}
	isoSem chan struct{} // separate semaphore for isochrone (2 concurrent)
}

// NewUnifiedRouter creates a router backed by any GraphReader.
// maxConcurrent limits simultaneous routing queries (semaphore-based).
func NewUnifiedRouter(reader GraphReader, maxConcurrent int) *UnifiedRouter {
	return &UnifiedRouter{
		reader:  reader,
		sem:     make(chan struct{}, maxConcurrent),
		isoSem:  make(chan struct{}, 2),
	}
}

// Route computes a route from start to destination using findRouteUnified
// (pass0 -> buildTrackIndexUnified -> pass1 -> retrackUnified).
func (r *UnifiedRouter) Route(ctx context.Context, req RouteRequest,
	cfg *CostConfig) (*RouteResult, error) {

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	totalStart := time.Now()

	src, _, ok := r.snapWithFallback(req.FromLat, req.FromLon, cfg)
	if !ok {
		return nil, fmt.Errorf("no graph node within 5km of start coordinate")
	}
	dst, _, ok := r.snapWithFallback(req.ToLat, req.ToLon, cfg)
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

	// Landmark-Heuristik initialisieren
	meta := r.reader.LandmarkMeta()
	setID := SelectLandmarkSet(cfg, meta)
	var landmarkH *LandmarkHeuristic
	if !altDisabled && meta != nil && meta.LandmarkCount > 0 {
		landmarkH = NewLandmarkHeuristic(r.reader, dst, setID)
	}

	// Bidirectional backward search (pruning boundary for Pass0)
	var bwd *BackwardResult
	if routingBidirectional {
		srcLat, srcLon, _ := r.reader.NodeCoord(src)
		dstLat, dstLon, _ := r.reader.NodeCoord(dst)
		euclidDist := geo.DistanceHaversine(
			float64(srcLat)/1e7, float64(srcLon)/1e7,
			float64(dstLat)/1e7, float64(dstLon)/1e7,
		)
		// Skip backward search for short routes — overhead > benefit under 10km
		if euclidDist >= 10_000 {
			initialUB := float32(euclidDist) * 2.0 * cfg.AvgCostPerKm / 1000.0
			bwdResult := RunBackwardSearch(r.reader, dst, cfg, initialUB)
			bwd = &bwdResult
			slog.Debug("backward search complete",
				"nodes_reached", len(bwd.LowerBound),
				"upper_bound", bwd.UpperBound)
		}
	}

	// Pass 0: inadmissible A* (passCoeff > 1), MIT Landmark-Heuristik
	res0, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
		cfg.Pass0Coefficient, 0, nil, landmarkH, bwd)
	if err != nil {
		return nil, fmt.Errorf("pass0: %w", err)
	}

	// Free backward map before Pass1 (~120 MB)
	bwd = nil

	// === Pass1: segmented or standard ===
	var result *RouteResult
	var pass1Exp int
	var pass1Ms int64

	if routingValleSeg {
		pass0Path := extractPath(res0.state, src, dst)
		segments := FindValleySegments(pass0Path, r.reader, 1000.0)

		if len(segments) >= 2 {
			// Route each segment independently and retrack per-segment.
			// This avoids the broken-parent-chain problem of merging states.
			var legs []*RouteResult
			allOK := true

			for _, seg := range segments {
				segTrackIdx := buildTrackIndexUnified(res0.state, seg.From, seg.To)
				segMax := segmentMaxCost(res0.state, seg.From, seg.To, cfg)
				segRes, segErr := findRouteUnified(ctx, r.reader, seg.From, seg.To, cfg,
					0, segMax, segTrackIdx, nil, nil)
				if segErr != nil {
					slog.Warn("segment failed, falling back",
						"from_idx", seg.FromIdx, "to_idx", seg.ToIdx, "error", segErr)
					allOK = false
					break
				}

				legResult := retrackUnified(r.reader, segRes.state, seg.From, seg.To)
				if legResult == nil {
					slog.Warn("segment retrack failed, falling back",
						"from_idx", seg.FromIdx, "to_idx", seg.ToIdx)
					allOK = false
					break
				}

				legs = append(legs, legResult)
				pass1Exp += segRes.expansions
				pass1Ms += segRes.durationMs
			}

			if allOK && len(legs) > 0 {
				result = mergeLegs(legs)
				slog.Info("segmented pass1 complete",
					"segments", len(segments),
					"expansions", pass1Exp,
					"duration_ms", pass1Ms)
			}
		}
	}

	// Unsegmented Pass1 (default or fallback)
	if result == nil {
		trackIndex := buildTrackIndexUnified(res0.state, src, dst)
		pass1MaxCost := res0.totalCost * cfg.Pass0Coefficient * cfg.Pass0Coefficient
		res1, err := findRouteUnified(ctx, r.reader, src, dst, cfg,
			0, pass1MaxCost, trackIndex, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("pass1: %w", err)
		}
		pass1Exp = res1.expansions
		pass1Ms = res1.durationMs

		result = retrackUnified(r.reader, res1.state, src, dst)
		if result == nil {
			return nil, fmt.Errorf("retrack failed: broken parent chain")
		}
	}
	result.PassStats = PassStats{
		Pass0Expansions: res0.expansions,
		Pass0DurationMs: res0.durationMs,
		Pass1Expansions: pass1Exp,
		Pass1DurationMs: pass1Ms,
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

// FindReachable finds all nodes reachable within the given cost budget.
// Uses the isochrone semaphore (2 concurrent) to limit resource usage.
func (r *UnifiedRouter) FindReachable(ctx context.Context,
	lat, lon float64, cfg *CostConfig, maxBudget float32, maxExpansions int,
) (map[uint64]reachableNode, bool, error) {

	select {
	case r.isoSem <- struct{}{}:
		defer func() { <-r.isoSem }()
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}

	src, _, ok := r.snapWithFallback(lat, lon, cfg)
	if !ok {
		return nil, false, fmt.Errorf("no graph node within 5km of coordinate")
	}

	nodes, truncated := findReachable(ctx, r.reader, src, cfg, maxBudget, maxExpansions)
	return nodes, truncated, nil
}

// ReachableNode is the exported type for isochrone/suggest consumers.
type ReachableNode struct {
	NodeID  uint64
	Cost    float32
	Lat     float64
	Lon     float64
	Unknown bool // Terra Incognita: incoming edge OSM-ID not in visited set
}

// Reader returns the underlying GraphReader.
func (r *UnifiedRouter) Reader() GraphReader { return r.reader }

// snapWithFallback snaps (lat, lon) to the nearest graph node that has at
// least one non-blocked outgoing edge for cfg. Uses NearestNodeFiltered on the
// underlying graph tile to skip motorway/trunk nodes that the profile blocks.
//
// This prevents coordinates near motorways from snapping onto blocked nodes,
// which would cause Pass0 to expand millions of nodes without ever reaching
// the destination (30s timeout).
func (r *UnifiedRouter) snapWithFallback(lat, lon float64, cfg *CostConfig) (uint64, float64, bool) {
	// First try unfiltered NearestNode (fast path — works when nearest node is usable).
	nodeID, dist, ok := r.reader.NearestNode(lat, lon)
	if ok && r.nodeHasUsableEdge(nodeID, cfg) {
		return nodeID, dist, true
	}

	// Nearest node is blocked (e.g. motorway). Use filtered spatial search
	// that skips nodes whose edges are all blocked by this profile.
	g := r.reader.GraphAt(lat, lon)
	if g == nil {
		return 0, 0, false
	}

	tileID := graph.TileIDFromCoord(lat, lon)
	localID, filteredDist := graph.NearestNodeFiltered(g, lat, lon, func(nid uint32) bool {
		globalID := MakeGlobalNodeID(tileID, nid)
		return r.nodeHasUsableEdge(globalID, cfg)
	})
	if filteredDist >= 65535 {
		return 0, 0, false
	}

	globalID := MakeGlobalNodeID(tileID, localID)
	distM := float64(filteredDist) / 10.0
	slog.Info("routing: profile-aware snap used",
		"lat", lat, "lon", lon,
		"node_id", globalID, "dist_m", distM)
	return globalID, distM, true
}

// nodeHasUsableEdge checks whether at least one outgoing edge from nodeID
// has a cost below costBlocked for the given profile.
func (r *UnifiedRouter) nodeHasUsableEdge(nodeID uint64, cfg *CostConfig) bool {
	usable := false
	r.reader.ForEachEdge(nodeID, func(ev EdgeView) {
		if usable {
			return
		}
		if cfg.EdgeCost(ev.Tags1, ev.Tags2).Flat < costBlocked {
			usable = true
		}
	})
	return usable
}

// FindReachableWithCoords returns reachable nodes with coordinates and
// Terra Incognita flags. Used by SuggestRoutes and Isochrone.
// Returns (nodes, truncated, error). truncated is true when the expansion
// limit (500k) was hit before the budget was exhausted.
func (r *UnifiedRouter) FindReachableWithCoords(ctx context.Context,
	lat, lon float64, cfg *CostConfig, maxBudget float32,
	visitedWayIDs map[int64]bool,
) ([]ReachableNode, bool, error) {

	nodes, truncated, err := r.FindReachable(ctx, lat, lon, cfg, maxBudget, 500000)
	if err != nil {
		return nil, false, err
	}

	result := make([]ReachableNode, 0, len(nodes))
	for nodeID, rn := range nodes {
		nodeLat, nodeLon, _ := r.reader.NodeCoord(nodeID)
		unknown := false
		if visitedWayIDs != nil && rn.EdgeHandle != 0 {
			osmID := r.reader.EdgeOSMID(rn.EdgeHandle)
			if osmID != 0 {
				unknown = !visitedWayIDs[osmID]
			}
		}
		result = append(result, ReachableNode{
			NodeID:  nodeID,
			Cost:    rn.Cost,
			Lat:     float64(nodeLat) / 1e7,
			Lon:     float64(nodeLon) / 1e7,
			Unknown: unknown,
		})
	}
	return result, truncated, nil
}

// extractPath reconstructs the node path from state by following parent pointers.
func extractPath(state *SparseRouterState, src, dst uint64) []uint64 {
	var path []uint64
	node := dst
	for node != src {
		path = append(path, node)
		ns, ok := state.nodes[node]
		if !ok {
			return nil
		}
		node = ns.Parent
		if len(path) > 10_000_000 {
			return nil
		}
	}
	path = append(path, src)
	slices.Reverse(path)
	return path
}

// segmentMaxCost computes the pass1 max cost for a segment.
func segmentMaxCost(state *SparseRouterState, from, to uint64, cfg *CostConfig) float32 {
	fromNS, ok1 := state.nodes[from]
	toNS, ok2 := state.nodes[to]
	if !ok1 || !ok2 {
		return 0
	}
	segCost := toNS.EffectiveCost - fromNS.EffectiveCost
	if segCost <= 0 {
		segCost = toNS.BestCost - fromNS.BestCost
	}
	// segCost <= 0 can occur on strongly descending segments where elevation
	// correction reduces EffectiveCost. In findRouteUnified, maxCost=0 means
	// "unlimited" — this is safe (no budget constraint for this segment).
	return segCost * cfg.Pass0Coefficient * cfg.Pass0Coefficient
}
