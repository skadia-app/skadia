// router.go — UnifiedRouter backed by any GraphReader.
package routing

import (
	"context"
	"fmt"
	"log/slog"
	"math"
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

// landmarkStrategy controls landmark selection. "legacy" = old BBox+pass+peak strategy.
// Default (empty) = crossing-flanks strategy.
var _ = os.Getenv("LANDMARK_STRATEGY") // read in landmark_unified.go

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
		reader: reader,
		sem:    make(chan struct{}, maxConcurrent),
		isoSem: make(chan struct{}, 2),
	}
}

// VirtualEndpoint represents a point projected onto a graph edge, with
// pre-computed costs and node states for the two adjacent real nodes.
type VirtualEndpoint struct {
	Proj    graph.EdgeProjection
	GlobalA uint64
	GlobalB uint64
	CostA   float32
	CostB   float32
	StateA  nodeState
	StateB  nodeState
	OrigLat float64
	OrigLon float64
}

// snapToEdge projects (lat, lon) onto the nearest passable graph edge and
// returns a VirtualEndpoint with pre-computed partial-edge costs to both
// adjacent nodes. This is the foundation for sub-edge start/destination
// placement (edge projection routing).
func (r *UnifiedRouter) snapToEdge(lat, lon float64, cfg *CostConfig) (*VirtualEndpoint, error) {
	filter := func(tags1 uint64) bool {
		return cfg.EdgeCost(tags1, 0).Flat < costBlocked
	}
	proj, ok := r.reader.FindNearestEdge(lat, lon, filter)
	if !ok {
		return nil, ErrNoRoute
	}

	ve := &VirtualEndpoint{
		Proj:    proj,
		GlobalA: MakeGlobalNodeID(proj.TileID, proj.NodeA),
		GlobalB: MakeGlobalNodeID(proj.TileID, proj.NodeB),
		OrigLat: lat,
		OrigLon: lon,
	}

	edgeCostFactors := cfg.EdgeCost(proj.EdgeTags1, 0)

	// Partial distances from projection point to each endpoint (in cm and mm).
	distA_cm := proj.FractionAB * proj.EdgeDistM * 100
	distB_cm := (1 - proj.FractionAB) * proj.EdgeDistM * 100

	// Elevation deltas from projection point to each endpoint (dm → mm).
	deltaEleA_mm := (proj.EleA_dm - proj.ProjEleDm) * 100
	deltaEleB_mm := (proj.EleB_dm - proj.ProjEleDm) * 100

	hystA := propagateHysteresis(PathState{}, deltaEleA_mm, int32(distA_cm)*10, cfg)
	hystB := propagateHysteresis(PathState{}, deltaEleB_mm, int32(distB_cm)*10, cfg)

	ve.CostA = float32(distA_cm)/100.0*edgeCostFactors.Flat + hystA.ElevCost
	ve.CostB = float32(distB_cm)/100.0*edgeCostFactors.Flat + hystB.ElevCost

	// Direction vectors from projection point to each endpoint.
	nodeALat, nodeALon, _ := r.reader.NodeCoord(ve.GlobalA)
	nodeBLat, nodeBLon, _ := r.reader.NodeCoord(ve.GlobalB)
	projLat7 := int32(proj.ProjLat * 1e7)
	projLon7 := int32(proj.ProjLon * 1e7)

	dxA, dyA := normalizeDir(float32(nodeALon-projLon7), float32(nodeALat-projLat7))
	dxB, dyB := normalizeDir(float32(nodeBLon-projLon7), float32(nodeBLat-projLat7))
	wayClass := edgeCostFactors.WayClass

	effA := ve.CostA + hystA.State.elevationCorrection(cfg)
	effB := ve.CostB + hystB.State.elevationCorrection(cfg)

	ve.StateA = nodeState{
		BestCost: ve.CostA, EffectiveCost: effA, FrontierCost: effA,
		PathState: hystA.State, LastEdgeDX: dxA, LastEdgeDY: dyA,
		LastWayClass: wayClass, EdgeHandle: proj.EdgeHandle, EdgeTags1: proj.EdgeTags1,
		Parent: noParent,
	}
	ve.StateB = nodeState{
		BestCost: ve.CostB, EffectiveCost: effB, FrontierCost: effB,
		PathState: hystB.State, LastEdgeDX: dxB, LastEdgeDY: dyB,
		LastWayClass: wayClass, EdgeHandle: proj.EdgeHandle, EdgeTags1: proj.EdgeTags1,
		Parent: noParent,
	}

	return ve, nil
}

// normalizeDir returns a unit-length direction vector. Returns (0,0) for zero-length input.
func normalizeDir(dx, dy float32) (float32, float32) {
	lenSq := dx*dx + dy*dy
	if lenSq > 0 {
		invLen := 1.0 / float32(math.Sqrt(float64(lenSq)))
		return dx * invLen, dy * invLen
	}
	return 0, 0
}

// routeSameEdge handles the special case where source and destination project
// to the same edge. The direct path on the edge is always optimal (triangle inequality).
func (r *UnifiedRouter) routeSameEdge(srcVE, dstVE *VirtualEndpoint, cfg *CostConfig) *RouteResult {
	proj := srcVE.Proj
	fracFrom := srcVE.Proj.FractionAB
	fracTo := dstVE.Proj.FractionAB

	// Determine travel direction: from source to destination (may go A→B or B→A).
	reversed := fracFrom > fracTo
	if reversed {
		fracFrom, fracTo = fracTo, fracFrom
	}

	directFraction := fracTo - fracFrom
	directDist := directFraction * proj.EdgeDistM

	// Use actual src/dst projection elevations — correct regardless of travel direction.
	srcEle := float64(srcVE.Proj.ProjEleDm) / 10.0
	dstEle := float64(dstVE.Proj.ProjEleDm) / 10.0
	var ascent, descent float64
	diff := dstEle - srcEle
	if diff > 0 {
		ascent = diff
	} else {
		descent = -diff
	}

	// Compute cost using slope-weighted edge cost factors and hysteresis.
	ecf := cfg.EdgeCost(proj.EdgeTags1, 0)
	distCm := directDist * 100
	// Elevation delta: from source projection to destination projection (in dm).
	// srcVE/dstVE already represent actual travel direction — no reversed branch needed.
	deltaEleDm := dstVE.Proj.ProjEleDm - srcVE.Proj.ProjEleDm
	hyst := propagateHysteresis(PathState{}, deltaEleDm*100, int32(distCm)*10, cfg)

	// Slope-weighted cost factor interpolation.
	cfUp := ecf.Up
	cfDown := ecf.Down
	if cfUp == 0 {
		cfUp = ecf.Flat
	}
	if cfDown == 0 {
		cfDown = ecf.Flat
	}
	flatW := float32(1.0) - hyst.UpWeight - hyst.DownWeight
	if flatW < 0 {
		flatW = 0
	}
	cf := cfUp*hyst.UpWeight + ecf.Flat*flatW + cfDown*hyst.DownWeight
	cost := float64(float32(distCm)/100.0*cf + hyst.ElevCost)

	coords := []LatLonEle{
		{Lat: srcVE.Proj.ProjLat, Lon: srcVE.Proj.ProjLon, Ele: float64(srcVE.Proj.ProjEleDm) / 10.0},
		{Lat: dstVE.Proj.ProjLat, Lon: dstVE.Proj.ProjLon, Ele: float64(dstVE.Proj.ProjEleDm) / 10.0},
	}

	return &RouteResult{
		Coordinates: coords,
		DistanceM:   directDist,
		AscentM:     ascent,
		DescentM:    descent,
		Cost:        cost,
	}
}

// Route computes a route from start to destination using edge projection
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

	// Edge projection: snap start/destination to nearest passable edge.
	srcVE, err := r.snapToEdge(req.FromLat, req.FromLon, cfg)
	if err != nil {
		return nil, fmt.Errorf("snap source: %w", err)
	}
	dstVE, err := r.snapToEdge(req.ToLat, req.ToLon, cfg)
	if err != nil {
		return nil, fmt.Errorf("snap destination: %w", err)
	}

	// Same-edge shortcut: if both endpoints project onto the same edge,
	// the direct on-edge path is always optimal (triangle inequality).
	if srcVE.Proj.EdgeHandle == dstVE.Proj.EdgeHandle {
		result := r.routeSameEdge(srcVE, dstVE, cfg)
		r.addApproach(result, srcVE, dstVE, cfg, true, true)
		result.PassStats.TotalDurationMs = time.Since(totalStart).Milliseconds()
		return result, nil
	}

	// Dual-destination landmark heuristic (admissible: min over both endpoints).
	meta := r.reader.LandmarkMeta()
	setID := SelectLandmarkSet(cfg, meta)
	var landmarkH *LandmarkHeuristic
	if !altDisabled && meta != nil && meta.LandmarkCount > 0 {
		landmarkH = NewDualLandmarkHeuristic(r.reader,
			dstVE.GlobalA, dstVE.GlobalB, dstVE.CostA, dstVE.CostB, setID)
	}

	// Bidirectional backward search with dual seeds (pruning boundary for Pass0).
	var bwd *BackwardResult
	if routingBidirectional {
		euclidDist := geo.DistanceHaversine(
			srcVE.OrigLat, srcVE.OrigLon,
			dstVE.OrigLat, dstVE.OrigLon,
		)
		// Skip backward search for short routes — overhead > benefit under 10km
		if euclidDist >= 10_000 {
			bwdStart := time.Now()
			initialUB := float32(euclidDist) * 2.0 * cfg.AvgCostPerKm / 1000.0
			bwdResult := RunBackwardSearch(r.reader,
				dstVE.GlobalA, dstVE.GlobalB, dstVE.CostA, dstVE.CostB,
				cfg, initialUB)
			bwd = &bwdResult
			slog.Info("backward search complete",
				"nodes_reached", len(bwd.LowerBound),
				"upper_bound", bwd.UpperBound,
				"duration_ms", time.Since(bwdStart).Milliseconds())
		}
	}

	// Pass 0: inadmissible A* (passCoeff > 1), with dual landmark heuristic.
	res0, err := findRouteUnified(ctx, r.reader, srcVE, dstVE, 0, 0, cfg,
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
		pass0Path := extractPath(res0.state, res0.bestDstNode)
		segments := FindValleySegments(pass0Path, r.reader, 1000.0)

		if len(segments) >= 2 {
			// Valley-seg: route each segment individually.
			// First segment starts at the projected source (srcVE); last segment ends at
			// the projected destination (dstVE). Interior segments use node-to-node (nil VEs).
			var legs []*RouteResult
			allOK := true

			for i, seg := range segments {
				// Pass edge-projection endpoints only at the route boundaries so that
				// the route starts/ends at the exact clicked coordinates, not at graph nodes.
				var segSrcVE, segDstVE *VirtualEndpoint
				if i == 0 {
					segSrcVE = srcVE // first segment starts at projected source
				}
				if i == len(segments)-1 {
					segDstVE = dstVE // last segment ends at projected destination
				}

				segTrackIdx := buildTrackIndexUnified(res0.state, seg.To, seg.From)
				segMax := segmentMaxCost(res0.state, seg.From, seg.To, cfg)
				segRes, segErr := findRouteUnified(ctx, r.reader, segSrcVE, segDstVE, seg.From, seg.To, cfg,
					0, segMax, segTrackIdx, nil, nil)
				if segErr != nil {
					slog.Warn("segment failed, falling back",
						"from_idx", seg.FromIdx, "to_idx", seg.ToIdx, "error", segErr)
					allOK = false
					break
				}

				legResult := retrackUnified(r.reader, segRes.state, segRes.bestDstNode, segSrcVE, segDstVE)
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

	// Unsegmented Pass1 (default or fallback from failed segmentation).
	if result == nil {
		var legErr error
		result, legErr = r.routeLegPass1(ctx, srcVE, dstVE, cfg, res0)
		if legErr != nil {
			return nil, legErr
		}
		pass1Exp = result.PassStats.Pass1Expansions
		pass1Ms = result.PassStats.Pass1DurationMs
	}

	result.PassStats = PassStats{
		Pass0Expansions: res0.expansions,
		Pass0DurationMs: res0.durationMs,
		Pass1Expansions: pass1Exp,
		Pass1DurationMs: pass1Ms,
		TotalDurationMs: time.Since(totalStart).Milliseconds(),
	}

	// Approach segments for edge-projected start/destination.
	r.addApproach(result, srcVE, dstVE, cfg, true, true)

	return result, nil
}

// RouteVia computes a route through multiple waypoints using edge projection.
// Hysteresis state is NOT transferred between legs (reset at via-points, like BRouter).
// Approach segments are only added at the first and last waypoint.
func (r *UnifiedRouter) RouteVia(ctx context.Context, waypoints []LatLon,
	cfg *CostConfig) (*RouteResult, error) {

	if len(waypoints) < 2 {
		return nil, fmt.Errorf("need at least 2 waypoints")
	}

	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	totalStart := time.Now()

	// Snap all waypoints once upfront.
	ves := make([]*VirtualEndpoint, len(waypoints))
	for i, wp := range waypoints {
		ve, err := r.snapToEdge(wp.Lat, wp.Lon, cfg)
		if err != nil {
			return nil, fmt.Errorf("waypoint %d: no passable edge within range", i)
		}
		ves[i] = ve
	}

	// Route each leg: pass0 + routeLegPass1.
	var legs []*RouteResult
	for i := range len(waypoints) - 1 {
		srcVE := ves[i]
		dstVE := ves[i+1]

		// Same-edge shortcut.
		if srcVE.Proj.EdgeHandle == dstVE.Proj.EdgeHandle {
			leg := r.routeSameEdge(srcVE, dstVE, cfg)
			legs = append(legs, leg)
			continue
		}

		// Dual landmark heuristic per leg.
		meta := r.reader.LandmarkMeta()
		setID := SelectLandmarkSet(cfg, meta)
		var landmarkH *LandmarkHeuristic
		if !altDisabled && meta != nil && meta.LandmarkCount > 0 {
			landmarkH = NewDualLandmarkHeuristic(r.reader,
				dstVE.GlobalA, dstVE.GlobalB, dstVE.CostA, dstVE.CostB, setID)
		}

		// Pass 0.
		res0, err := findRouteUnified(ctx, r.reader, srcVE, dstVE, 0, 0, cfg,
			cfg.Pass0Coefficient, 0, nil, landmarkH, nil)
		if err != nil {
			return nil, fmt.Errorf("leg %d pass0: %w", i, err)
		}

		// Pass 1 + retrack.
		leg, err := r.routeLegPass1(ctx, srcVE, dstVE, cfg, res0)
		if err != nil {
			return nil, fmt.Errorf("leg %d: %w", i, err)
		}
		legs = append(legs, leg)
	}

	result := mergeLegs(legs)

	// Approach segments only at first and last waypoint.
	r.addApproach(result, ves[0], ves[len(ves)-1], cfg, true, true)

	// Aggregate pass stats.
	result.PassStats.TotalDurationMs = time.Since(totalStart).Milliseconds()
	return result, nil
}

// FindReachable finds all nodes reachable within the given cost budget.
// Uses the isochrone semaphore (2 concurrent) to limit resource usage.
func (r *UnifiedRouter) FindReachable(ctx context.Context,
	lat, lon float64, cfg *CostConfig, maxBudget float32, maxExpansions int,
) (map[uint64]reachableNodeLegacy, bool, error) {

	select {
	case r.isoSem <- struct{}{}:
		defer func() { <-r.isoSem }()
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}

	src, _, ok := r.snapToNode(lat, lon, cfg)
	if !ok {
		return nil, false, fmt.Errorf("no graph node within 5km of coordinate")
	}

	nodes, truncated := findReachableLegacy(ctx, r.reader, src, cfg, maxBudget, maxExpansions)
	return nodes, truncated, nil
}

// ReachableNode is the exported type for isochrone/suggest consumers.
type ReachableNode struct {
	NodeID  uint64
	Cost    float32
	Lat     float64
	Lon     float64
	Unknown bool // Terra Incognita: incoming edge OSM-ID not in visited set
	// Path-averaged coverage scores, normalized [0, 1]
	ForestPct      float32
	NoiseScore     float32
	ProtectedArea  float32
	Urbanization   float32
	RiverProximity float32
	TrafficClass   float32
}

// Reader returns the underlying GraphReader.
func (r *UnifiedRouter) Reader() GraphReader { return r.reader }

// snapToNode snaps (lat, lon) to the nearest graph node that has at
// least one non-blocked outgoing edge for cfg. Uses NearestNodeFiltered on the
// underlying graph tile to skip motorway/trunk nodes that the profile blocks.
//
// Used by FindReachable/FindReachableWithCoords (isochrone). For routing,
// use snapToEdge instead (edge projection).
func (r *UnifiedRouter) snapToNode(lat, lon float64, cfg *CostConfig) (uint64, float64, bool) {
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
			NodeID:         nodeID,
			Cost:           rn.Cost,
			Lat:            float64(nodeLat) / 1e7,
			Lon:            float64(nodeLon) / 1e7,
			Unknown:        unknown,
			ForestPct:      rn.ForestPct,
			NoiseScore:     rn.NoiseScore,
			ProtectedArea:  rn.ProtectedArea,
			Urbanization:   rn.Urbanization,
			RiverProximity: rn.RiverProximity,
			TrafficClass:   rn.TrafficClass,
		})
	}
	return result, truncated, nil
}

// routeLegPass1 runs Pass1 + Retrack for a single leg given a Pass0 result.
func (r *UnifiedRouter) routeLegPass1(ctx context.Context,
	srcVE, dstVE *VirtualEndpoint, cfg *CostConfig,
	res0 *routeResult,
) (*RouteResult, error) {
	trackIndex := buildTrackIndexUnified(res0.state, res0.bestDstNode, 0)
	pass1MaxCost := res0.totalCost * cfg.Pass0Coefficient * cfg.Pass0Coefficient
	res1, err := findRouteUnified(ctx, r.reader, srcVE, dstVE, 0, 0, cfg,
		0, pass1MaxCost, trackIndex, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pass1: %w", err)
	}
	result := retrackUnified(r.reader, res1.state, res1.bestDstNode, srcVE, dstVE)
	if result == nil {
		return nil, fmt.Errorf("retrack failed: broken parent chain")
	}
	result.PassStats.Pass1Expansions = res1.expansions
	result.PassStats.Pass1DurationMs = res1.durationMs
	return result, nil
}

// addApproach adds approach segments (straight lines) to a result when the
// snap distance from the original coordinate to the edge projection exceeds
// a threshold. This visualizes the gap between where the user clicked and
// where the route actually starts/ends on the graph.
func (r *UnifiedRouter) addApproach(result *RouteResult, srcVE, dstVE *VirtualEndpoint,
	cfg *CostConfig, includeFrom, includeTo bool) {
	snapThreshold := float64(100)
	if cfg.SnapThresholdM > 0 {
		snapThreshold = float64(cfg.SnapThresholdM)
	}
	if includeFrom && srcVE.Proj.DistanceM > snapThreshold {
		result.ApproachFrom = []LatLonEle{
			{Lat: srcVE.OrigLat, Lon: srcVE.OrigLon, Ele: float64(srcVE.Proj.ProjEleDm) / 10.0},
			{Lat: srcVE.Proj.ProjLat, Lon: srcVE.Proj.ProjLon, Ele: float64(srcVE.Proj.ProjEleDm) / 10.0},
		}
	}
	if includeTo && dstVE.Proj.DistanceM > snapThreshold {
		result.ApproachTo = []LatLonEle{
			{Lat: dstVE.Proj.ProjLat, Lon: dstVE.Proj.ProjLon, Ele: float64(dstVE.Proj.ProjEleDm) / 10.0},
			{Lat: dstVE.OrigLat, Lon: dstVE.OrigLon, Ele: float64(dstVE.Proj.ProjEleDm) / 10.0},
		}
	}
}

// extractPath reconstructs the node path from state by following parent pointers.
// Terminates at the source seed node(s) stored in state.
func extractPath(state *SparseRouterState, dst uint64) []uint64 {
	var path []uint64
	node := dst
	for node != state.srcSeedA && node != state.srcSeedB {
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
	path = append(path, node)
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
