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

	"github.com/skadia-app/skadia/services/api/internal/dem"
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

// ReachableState is an exported alias for reachableState so that callers
// outside the routing package (e.g. handler) can hold a reference returned
// by FindReachableWithCoords without accessing unexported internals.
type ReachableState = reachableState

// UnifiedRouter is a Router backed by any GraphReader (e.g. TiledGraphReader).
// Uses a semaphore for concurrency control instead of a pre-allocated pool.
type UnifiedRouter struct {
	reader GraphReader
	sem    chan struct{}
	isoSem chan struct{} // separate semaphore for isochrone (2 concurrent)
	demSvc dem.Servicer // injected at boot for off-graph DEM lookups (Spec §3.2)
}

// NewUnifiedRouter creates a router backed by any GraphReader.
// maxConcurrent limits simultaneous routing queries (semaphore-based).
// maxConcurrentIsochrones limits concurrent isochrone/reachability expansions
// (defaults to 2 if <= 0; override via SKADIA_ISO_CONCURRENCY env).
// demSvc is used for off-graph DEM lookups (approach/departure elevation profiles).
// Pass nil to disable DEM lookups (useful in tests).
func NewUnifiedRouter(reader GraphReader, maxConcurrent int, maxConcurrentIsochrones int, demSvc dem.Servicer) *UnifiedRouter {
	if maxConcurrentIsochrones <= 0 {
		maxConcurrentIsochrones = 2
	}
	return &UnifiedRouter{
		reader: reader,
		sem:    make(chan struct{}, maxConcurrent),
		isoSem: make(chan struct{}, maxConcurrentIsochrones),
		demSvc: demSvc,
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
//
// T0 Phase C.4 (Spec §5.8): Coords AND ElevationProfile are sliced parallel
// from the full-resolution edge data via sliceEdgeCoordsAndProfile. The unified
// vertex set guarantees len(Coordinates) == len(ElevationProfile). Ascent/
// descent are integrated over the sliced profile so non-monotonic edges
// (Stichweg/Refugio approach with intermediate humps) report correct elevation
// gain instead of the previous endpoint-only delta.
func (r *UnifiedRouter) routeSameEdge(srcVE, dstVE *VirtualEndpoint, cfg *CostConfig) *RouteResult {
	proj := srcVE.Proj
	fracFrom := srcVE.Proj.FractionAB
	fracTo := dstVE.Proj.FractionAB

	// Determine travel direction. fStart/fEnd are the canonical (ascending)
	// fraction bounds passed to the slicer; reversed = traveler walks B→A.
	reversed := fracFrom > fracTo
	fStart, fEnd := fracFrom, fracTo
	if reversed {
		fStart, fEnd = fracTo, fracFrom
	}

	// Build the master full-edge coord polyline: [NodeA, ...intermediates..., NodeB].
	nodeALat, nodeALon, _ := r.reader.NodeCoord(srcVE.GlobalA)
	nodeBLat, nodeBLon, _ := r.reader.NodeCoord(srcVE.GlobalB)
	intermLats, intermLons := r.reader.EdgeGeometry(proj.EdgeHandle)
	coordsRaw := make([][2]int32, 0, len(intermLats)+2)
	coordsRaw = append(coordsRaw, [2]int32{nodeALat, nodeALon})
	for i := range intermLats {
		coordsRaw = append(coordsRaw, [2]int32{intermLats[i], intermLons[i]})
	}
	coordsRaw = append(coordsRaw, [2]int32{nodeBLat, nodeBLon})

	// Build the master profile from raw EdgeElevProf data (cm → m, NoData → nil).
	elevsDm, segDistsCm := r.reader.EdgeElevProfile(proj.EdgeHandle)
	edgeProf := buildEdgeProfFromRaw(elevsDm, segDistsCm)

	var slicedCoords []LatLonEle
	var slicedProfile []ElevationPoint
	if len(edgeProf) > 0 {
		slicedCoords, slicedProfile = sliceEdgeCoordsAndProfile(coordsRaw, edgeProf, fStart, fEnd)
	}

	usedFallback := false
	// Defensive: empty edge data (no DEM profile, single-vertex polyline). Fall
	// back to projection-point endpoints so the route still has at least the
	// two clicked positions. Profile uses ProjEleDm (best available signal).
	// The fallback is built directly in src→dst order, so we skip the reverse
	// step for it (no canonical ↔ traversal direction transformation needed).
	if len(slicedCoords) == 0 {
		usedFallback = true
		slicedCoords = []LatLonEle{
			{Lat: srcVE.Proj.ProjLat, Lon: srcVE.Proj.ProjLon, Ele: float64(srcVE.Proj.ProjEleDm) / 10.0, EleValid: true},
			{Lat: dstVE.Proj.ProjLat, Lon: dstVE.Proj.ProjLon, Ele: float64(dstVE.Proj.ProjEleDm) / 10.0, EleValid: true},
		}
		fallbackDist := math.Abs(fEnd-fStart) * proj.EdgeDistM
		if fallbackDist <= 1e-6 {
			// Degenerate same-edge case: src and dst project to the same fraction
			// (e.g. user clicked the same point twice). A two-point profile with
			// DistM=[0,0] would fail IsT0FormCompliant's strict-monotonicity check.
			// Emit a single anchor point instead — zero-distance route, one position.
			slicedProfile = []ElevationPoint{
				{DistM: 0, EleM: PtrEle(float64(srcVE.Proj.ProjEleDm) / 10.0)},
			}
		} else {
			slicedProfile = []ElevationPoint{
				{DistM: 0, EleM: PtrEle(float64(srcVE.Proj.ProjEleDm) / 10.0)},
				{DistM: fallbackDist, EleM: PtrEle(float64(dstVE.Proj.ProjEleDm) / 10.0)},
			}
		}
	}

	if reversed && !usedFallback {
		slicedCoords = reverseLatLonEle(slicedCoords)
		slicedProfile = reverseElevSlice(slicedProfile)
	}

	// directDist = master truth from the haversine cumsum (sliced profile's
	// last DistM). Falls back to the projection-fraction span on the rare
	// fallback path where the profile was synthesized from ProjEleDm above.
	directDist := slicedProfile[len(slicedProfile)-1].DistM
	if directDist <= 0 {
		directDist = math.Abs(fEnd-fStart) * proj.EdgeDistM
	}

	// Integrate ascent/descent over the full sliced profile. Skip segments
	// touching nil EleM samples (NoData) — gap-fill phase D will resolve.
	var ascent, descent float64
	for i := 1; i < len(slicedProfile); i++ {
		if slicedProfile[i].EleM == nil || slicedProfile[i-1].EleM == nil {
			continue
		}
		diff := *slicedProfile[i].EleM - *slicedProfile[i-1].EleM
		if diff > 0 {
			ascent += diff
		} else {
			descent -= diff
		}
	}

	// Compute cost using slope-weighted edge cost factors and hysteresis.
	// Per-segment hysteresis integration is a follow-up (out of T0 scope);
	// for now use the existing single-segment shape but driven by the sliced
	// directDist + endpoint-elevation delta in dm. The endpoint elevations
	// come from the sliced profile (master truth) when available, falling
	// back to ProjEleDm (DEM-rounded but consistent with the snap helper).
	ecf := cfg.EdgeCost(proj.EdgeTags1, 0)
	distCm := directDist * 100

	startEleM := slicedProfile[0].EleM
	endEleM := slicedProfile[len(slicedProfile)-1].EleM
	var deltaEleDm int32
	if startEleM != nil && endEleM != nil {
		deltaEleDm = int32(math.Round((*endEleM - *startEleM) * 10))
	} else {
		// NoData on either endpoint — fall back to projection elevations.
		deltaEleDm = dstVE.Proj.ProjEleDm - srcVE.Proj.ProjEleDm
	}
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

	return &RouteResult{
		Coordinates:    slicedCoords,
		DistanceM:      directDist,
		TotalDistanceM: directDist, // same-edge: approach-free at this stage; addApproach overrides
		AscentM:        ascent,
		DescentM:       descent,
		Cost:           cost,
		ElevationProfile: &slicedProfile, // Spec §5.8, T1.3 — full-resolution profile (T0 C.4)
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

				legResult := retrackUnified(r.reader, segRes.state, segRes.bestDstNode, segSrcVE, segDstVE, cfg)
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

// FindReachable finds all nodes reachable within the given cost budget
// using Pareto-Dijkstra over (cost, ascent). Returns the full reachableState
// (for later reconstruction) + truncated flag + error.
func (r *UnifiedRouter) FindReachable(ctx context.Context,
	lat, lon float64, cfg *CostConfig, maxBudget float32,
) (*reachableState, bool, error) {

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

	rs, err := findReachable(ctx, r.reader, src, cfg, maxBudget)
	if err != nil {
		return rs, rs != nil && rs.truncated, err
	}
	return rs, rs.truncated, nil
}

// NodePareto is a single Pareto-optimal path to a reachable node with all
// of its per-state metrics and per-state coverage signature.
type NodePareto struct {
	// Pareto axes
	Cost   float32
	Ascent float32

	// Metrics
	Descent   float32
	DistanceM float32
	AvgFlat   float32

	// Per-state coverage (distance-weighted averages along the path)
	ForestPct      float32
	NoiseScore     float32
	ProtectedArea  float32
	Urbanization   float32
	RiverProximity float32
	TrafficClass   float32
}

// ReachableNode is the exported representation of a reachable node, built
// after findReachable completes from the active pareto front of each node.
type ReachableNode struct {
	NodeID  uint64
	Lat     float64
	Lon     float64
	Unknown bool // Terra Incognita: incoming edge OSM-ID not in visited set

	// Pareto-optimal states, sorted by cost ascending.
	States []NodePareto

	// Slab indices corresponding 1:1 to States[] — used by variant selection
	// to drive ReconstructPath without public API leakage.
	StateSlabIDs []uint32

	// Deprecated: Cost is an alias for States[0].Cost, preserved so that
	// isochrone handlers don't require code changes during the feature-flag
	// rollout window.
	Cost float32

	// Deprecated: coverage aliases for backward compat with isochrone.
	// Set from States[0] at export time.
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

// FindReachableWithCoords returns reachable nodes with per-state Pareto
// coverage plus Terra Incognita flags. Used by SuggestRoutes and Isochrone.
func (r *UnifiedRouter) FindReachableWithCoords(ctx context.Context,
	lat, lon float64, cfg *CostConfig, maxBudget float32,
	visitedWayIDs map[int64]bool,
) ([]ReachableNode, *reachableState, bool, error) {

	rs, truncated, err := r.FindReachable(ctx, lat, lon, cfg, maxBudget)
	if err != nil {
		return nil, rs, truncated, err
	}

	result := make([]ReachableNode, 0, len(rs.fronts))
	for nodeID, front := range rs.fronts {
		if len(front.activeIDs) == 0 {
			continue
		}

		nodeLat, nodeLon, _ := r.reader.NodeCoord(nodeID)

		states := make([]NodePareto, len(front.activeIDs))
		slabIDs := make([]uint32, len(front.activeIDs))
		for i, slabID := range front.activeIDs {
			s := rs.slab.states[slabID]
			slabIDs[i] = slabID
			states[i] = NodePareto{
				Cost:      s.cost,
				Ascent:    s.ascent,
				Descent:   s.descent,
				DistanceM: s.dist,
				AvgFlat:   paretoAvgFlat(s),

				ForestPct:      paretoCovNorm(s.covForestSum, s.covDistSum),
				NoiseScore:     paretoCovNorm(s.covNoiseSum, s.covDistSum),
				ProtectedArea:  paretoCovNorm(s.covProtectedSum, s.covDistSum),
				Urbanization:   paretoCovNorm(s.covUrbanSum, s.covDistSum),
				RiverProximity: paretoCovNorm(s.covRiverSum, s.covDistSum),
				TrafficClass:   paretoCovNorm(s.covTrafficSum, s.covDistSum),
			}
		}

		minCostState := rs.slab.states[front.activeIDs[0]]
		unknown := false
		if visitedWayIDs != nil && minCostState.handle != 0 {
			osmID := r.reader.EdgeOSMID(minCostState.handle)
			if osmID != 0 {
				unknown = !visitedWayIDs[osmID]
			}
		}

		// Set deprecated aliases from States[0] for backward compat
		rn := ReachableNode{
			NodeID:       nodeID,
			Lat:          float64(nodeLat) / 1e7,
			Lon:          float64(nodeLon) / 1e7,
			Unknown:      unknown,
			States:       states,
			StateSlabIDs: slabIDs,
			Cost:         minCostState.cost,
			ForestPct:    states[0].ForestPct,
			NoiseScore:   states[0].NoiseScore,
			ProtectedArea: states[0].ProtectedArea,
			Urbanization: states[0].Urbanization,
			RiverProximity: states[0].RiverProximity,
			TrafficClass: states[0].TrafficClass,
		}
		result = append(result, rn)
	}

	return result, rs, truncated, nil
}

func paretoAvgFlat(s paretoState) float32 {
	if s.covDistSum == 0 {
		return 0
	}
	return s.qualitySum / s.covDistSum
}

func paretoCovNorm(sum, dist float32) float32 {
	if dist == 0 {
		return 0
	}
	return sum / dist
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
	result := retrackUnified(r.reader, res1.state, res1.bestDstNode, srcVE, dstVE, cfg)
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
//
// When r.demSvc is non-nil, click and projection elevations are resolved via
// dem.Servicer.ElevationBatch (4-point batch: clickSrc, projSrc, projDst, clickDst).
// This fixes the previous bug where both the click and projection coordinates
// received the projection-point elevation (ProjEleDm) — causing an incorrect
// elevation at the user-clicked map position (Spec §6 Class A correction).
//
// Additionally builds ApproachFromProfile / ApproachToProfile (2-point headers)
// and prepends/appends them to result.ElevationProfile, making the combined
// profile approach-inclusive: Profile[0].DistM==0 (click_src),
// Profile[last].DistM≈result.DistanceM (click_dst).
func (r *UnifiedRouter) addApproach(result *RouteResult, srcVE, dstVE *VirtualEndpoint,
	cfg *CostConfig, includeFrom, includeTo bool) {
	snapThreshold := float64(100)
	if cfg.SnapThresholdM > 0 {
		snapThreshold = float64(cfg.SnapThresholdM)
	}

	// Resolve DEM elevations for all four approach points in a single batch.
	// Order: [clickSrc, projSrc, projDst, clickDst].
	// Falls back to projection elevation (ProjEleDm) when r.demSvc is nil
	// (test paths) — EleValid is false in that case.
	var srcClickEle, srcProjEle, dstProjEle, dstClickEle *float64
	if r.demSvc != nil {
		latsE7 := []int32{
			int32(math.Round(srcVE.OrigLat * 1e7)),
			int32(math.Round(srcVE.Proj.ProjLat * 1e7)),
			int32(math.Round(dstVE.Proj.ProjLat * 1e7)),
			int32(math.Round(dstVE.OrigLat * 1e7)),
		}
		lonsE7 := []int32{
			int32(math.Round(srcVE.OrigLon * 1e7)),
			int32(math.Round(srcVE.Proj.ProjLon * 1e7)),
			int32(math.Round(dstVE.Proj.ProjLon * 1e7)),
			int32(math.Round(dstVE.OrigLon * 1e7)),
		}
		eles, err := r.demSvc.ElevationBatch(latsE7, lonsE7)
		if err != nil {
			slog.Warn("dem.ElevationBatch failed for approach", "err", err)
		} else if len(eles) == 4 {
			srcClickEle = approachElevToPtrM(eles[0])
			srcProjEle = approachElevToPtrM(eles[1])
			dstProjEle = approachElevToPtrM(eles[2])
			dstClickEle = approachElevToPtrM(eles[3])
		}
	}

	var approachFromM, approachToM float64
	if includeFrom && srcVE.Proj.DistanceM > snapThreshold {
		approachFromM = geo.DistanceHaversine(
			srcVE.OrigLat, srcVE.OrigLon,
			srcVE.Proj.ProjLat, srcVE.Proj.ProjLon,
		)
		result.ApproachFrom = []LatLonEle{
			{
				Lat:      srcVE.OrigLat,
				Lon:      srcVE.OrigLon,
				Ele:      approachDerefOrZero(srcClickEle),
				EleValid: srcClickEle != nil,
			},
			{
				Lat:      srcVE.Proj.ProjLat,
				Lon:      srcVE.Proj.ProjLon,
				Ele:      approachDerefOrZero(srcProjEle),
				EleValid: srcProjEle != nil,
			},
		}
		result.ApproachFromProfile = []ElevationPoint{
			{DistM: 0, EleM: srcClickEle},
			{DistM: approachFromM, EleM: srcProjEle},
		}
	}
	if includeTo && dstVE.Proj.DistanceM > snapThreshold {
		approachToM = geo.DistanceHaversine(
			dstVE.Proj.ProjLat, dstVE.Proj.ProjLon,
			dstVE.OrigLat, dstVE.OrigLon,
		)
		result.ApproachTo = []LatLonEle{
			{
				Lat:      dstVE.Proj.ProjLat,
				Lon:      dstVE.Proj.ProjLon,
				Ele:      approachDerefOrZero(dstProjEle),
				EleValid: dstProjEle != nil,
			},
			{
				Lat:      dstVE.OrigLat,
				Lon:      dstVE.OrigLon,
				Ele:      approachDerefOrZero(dstClickEle),
				EleValid: dstClickEle != nil,
			},
		}
		result.ApproachToProfile = []ElevationPoint{
			{DistM: 0, EleM: dstProjEle},
			{DistM: approachToM, EleM: dstClickEle},
		}
	}

	// Extend result.ElevationProfile to be approach-inclusive:
	//   [ApproachFromProfile] + [existing network+VE-tail profile] + [ApproachToProfile]
	// This makes Profile[0].DistM==0 at click_src and
	// Profile[last].DistM≈result.DistanceM at click_dst.
	//
	// Only prepend/append when the corresponding approach was added (non-nil profile).
	// Uses Variant (b): Approach-Coord elevations are already set above; the
	// combined profile is purely for the JSON elevation_profile response.
	hasFromProfile := len(result.ApproachFromProfile) > 0
	hasToProfile := len(result.ApproachToProfile) > 0
	if hasFromProfile || hasToProfile {
		var netProfile []ElevationPoint
		if result.ElevationProfile != nil {
			netProfile = *result.ElevationProfile
		}
		combined := concatProfilesWithJunctionDedup(
			result.ApproachFromProfile,
			netProfile,
			result.ApproachToProfile,
		)
		if len(combined) > 0 {
			result.ElevationProfile = &combined
		}
	}

	// Approach-inclusive total distance for the response. result.DistanceM stays
	// network-only — used by cost/stat consumers; do not mutate.
	//
	// Note: concatProfilesWithJunctionDedup removes the shared boundary point
	// between ApproachFrom and the network profile (and between the network
	// profile and ApproachTo). When approach_from_m is very small, this dedup
	// compresses the first few meters visually — the chart appears to "start"
	// slightly into the approach. This is by design: the junction point is
	// identical in both segments, so keeping it would produce a duplicate DistM
	// which fails the strict-monotonicity invariant. Cosmetic artefact only.
	if result.ElevationProfile != nil && len(*result.ElevationProfile) > 0 {
		result.TotalDistanceM = (*result.ElevationProfile)[len(*result.ElevationProfile)-1].DistM
	} else {
		// Fallback: no profile means we cannot compute the inclusive distance;
		// expose network-only distance to keep the field defined.
		result.TotalDistanceM = result.DistanceM
	}
}

// approachElevToPtrM converts a DEM elevation in decimeters to *float64 in meters.
// Returns nil for the NoData sentinel (dem.NoData = -32768), which signals missing DEM coverage.
func approachElevToPtrM(e int32) *float64 {
	if e == dem.NoData {
		return nil
	}
	v := float64(e) / 10.0
	return &v
}

// approachDerefOrZero dereferences a *float64 elevation, returning 0.0 when nil.
// Used to populate LatLonEle.Ele when EleValid tracks validity separately.
func approachDerefOrZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
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
