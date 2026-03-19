// unified.go — Unified A* routing with GraphReader interface.
// Replaces: pass0, pass1, pass0Sparse, pass1Sparse, retrack, retrackSparse.
// One findRouteUnified handles both pass0 (inadmissible A*, c>1) and pass1 (Dijkstra, c=0).
// Spec: §9.1-9.5
package routing

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// nodeState holds per-node routing state in the sparse map.
// Value-type (not pointer) to reduce GC pressure (C19).
// ~64 bytes padded — small enough for value-type map semantics.
type nodeState struct {
	BestCost      float32   // g(n) — raw accumulated cost
	EffectiveCost float32   // g(n) + elevationCorrection — for closed-set comparison (C01)
	FrontierCost  float32   // f(n) = EffectiveCost + passCoeff*h — used for stale-entry detection
	Parent        uint64    // global node ID of predecessor
	EdgeHandle    uint64    // for retrack: which edge led to this node
	EdgeTags1     uint64    // A09: Tags1 of incoming edge — avoids O(degree) ForEachEdge in retrack
	PathState     PathState // K14: hysteresis buffers (ehbu, ehbd int32)
	LastEdgeDX    float32   // R01: NORMALIZED direction vector from computeTurnCost
	LastEdgeDY    float32   // R01: unit length for dot-product cosine in next expansion
	LastWayClass  uint8     // WayClass of last edge (for initialcost tracking)
}

// SparseRouterState uses a value-type map for unknown node counts.
// C19: map[uint64]nodeState (value-type, not pointer) — eliminates per-entry
// heap allocation and GC pressure.
type SparseRouterState struct {
	nodes map[uint64]nodeState
}

func newSparseRouterState(hint int) *SparseRouterState {
	return &SparseRouterState{nodes: make(map[uint64]nodeState, hint)}
}

// routeResult is the internal result of a single findRouteUnified call.
type routeResult struct {
	state      *SparseRouterState
	totalCost  float32
	expansions int
	durationMs int64
}

const (
	unifiedMaxExpansions = 2_000_000
	unifiedCheckCancel   = 4096
)

// findRouteUnified — unified A* for pass0 and pass1.
//
// passCoeff > 1.0 for pass0 (inadmissible A*), 0 for pass1 (Dijkstra).
// trackIndex: nil for pass0, map from pass0 path for pass1 cost-cutting (R06).
// maxCost: 0 for pass0 (unlimited), pass0 totalCost for pass1.
//
// Returns routeResult with state, totalCost, expansions, and duration.
func findRouteUnified(ctx context.Context, reader GraphReader,
	src, dst uint64, cfg *CostConfig,
	passCoeff float32, maxCost float32,
	trackIndex map[uint64]float32,
) (*routeResult, error) {

	start := time.Now()
	state := newSparseRouterState(8192) // C19: value-type map
	frontier := Frontier{entries: make([]FrontierEntry, 0, 8192)} // K17: capacity hint

	dstLat, dstLon, _ := reader.NodeCoord(dst)

	// Seed source node
	state.nodes[src] = nodeState{BestCost: 0, EffectiveCost: 0, FrontierCost: 0}
	frontier.Push(FrontierEntry{NodeID: src, Cost: 0})

	expansions := 0

	for frontier.Len() > 0 {
		// Periodic context cancellation check
		if expansions&(unifiedCheckCancel-1) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		expansions++
		if expansions > unifiedMaxExpansions {
			return nil, fmt.Errorf("max expansions exceeded (%d)", unifiedMaxExpansions)
		}

		entry := frontier.Pop()
		ns, ok := state.nodes[entry.NodeID]
		if !ok {
			continue // should not happen, but guard
		}

		// Stale-entry check: skip if a better path was already found.
		// A04/A*: entry.Cost is f(n) = g(n) + passCoeff*h(n); compare against
		// the last pushed FrontierCost so the heuristic does not cause false staleness.
		if entry.Cost > ns.FrontierCost+0.001 {
			continue
		}

		// Reached destination — return result.
		// totalCost = EffectiveCost (includes elevationCorrection) because
		// pass1 uses it as maxCost bound against EffectiveCost comparisons.
		if entry.NodeID == dst {
			return &routeResult{
				state:      state,
				totalCost:  ns.EffectiveCost,
				expansions: expansions,
				durationMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// Cost-cutting via trackIndex (pass1 only).
		// R06: remaining scaled by Pass0Coefficient for safety margin.
		// Pass0 is inadmissible (c>1) so its track costs are distorted —
		// remaining can underestimate actual remaining cost by 15-20%.
		cutoff := maxCost
		if trackIndex != nil {
			if remaining, onTrack := trackIndex[entry.NodeID]; onTrack {
				tightened := ns.EffectiveCost + remaining*cfg.Pass0Coefficient
				if tightened < cutoff || cutoff == 0 {
					cutoff = tightened
				}
			}
		}
		if cutoff > 0 && ns.EffectiveCost > cutoff {
			continue
		}

		// Current node coordinates (once per expansion, for turn cost direction)
		curLat, curLon, _ := reader.NodeCoord(entry.NodeID)

		reader.ForEachEdge(entry.NodeID, func(ev EdgeView) {
			ecf := cfg.EdgeCost(ev.Tags1, ev.Tags2)
			if ecf.Flat >= costBlocked {
				return // impassable
			}

			// Hysteresis: elevation cost + slope weights
			var newPathState PathState
			var elevCost, upWeight, downWeight float32

			if passCoeff > 1.0 {
				// K04: Pass0 (inadmissible, c>1.0): simple DeltaEle (fast)
				hr := propagateHysteresis(ns.PathState, ev.DeltaEle*100, int32(ev.Dist)*10, cfg)
				newPathState = hr.State
				elevCost = hr.ElevCost
				upWeight = hr.UpWeight
				downWeight = hr.DownWeight
			} else {
				// Pass1: full elevation profile (accurate)
				elevs, segDistsCm := reader.EdgeElevProfile(ev.ElevHandle)
				if len(segDistsCm) > 0 {
					hr := propagateHysteresisProfileCm(ns.PathState, elevs, segDistsCm, cfg)
					newPathState = hr.State
					elevCost = hr.ElevCost
					upWeight = hr.UpWeight
					downWeight = hr.DownWeight
				} else {
					// Fallback: no segment distances (degenerate edge or missing profile)
					hr := propagateHysteresis(ns.PathState, ev.DeltaEle*100, int32(ev.Dist)*10, cfg)
					newPathState = hr.State
					elevCost = hr.ElevCost
					upWeight = hr.UpWeight
					downWeight = hr.DownWeight
				}
			}

			// Slope-weighted costfactor interpolation (BRouter-exact)
			cfUp := ecf.Up
			cfDown := ecf.Down
			if cfUp == 0 {
				cfUp = ecf.Flat
			}
			if cfDown == 0 {
				cfDown = ecf.Flat
			}
			flatW := float32(1.0) - upWeight - downWeight
			if flatW < 0 {
				flatW = 0
			}
			cf := cfUp*upWeight + ecf.Flat*flatW + cfDown*downWeight
			distCost := float32(ev.Dist) / 100.0 * cf // cm -> m

			// K03: Initialcost (WayClass transition penalty)
			// Approximation: cfg.TurnCost * 2 for any way class change.
			// Full InitialcostForWayClass per Spec §6.5 deferred to Stufe 3b.
			var initialCost float32
			if ns.LastWayClass != ecf.WayClass && ns.LastWayClass > 0 {
				// Ferry transition gets heavier penalty (BRouter: 10km equivalent)
				if ecf.WayClass == 1 || ns.LastWayClass == 1 {
					initialCost = 10000
				} else {
					initialCost = cfg.TurnCost * 2
				}
			}

			// R01: TurnCost — computeTurnCost returns NORMALIZED newDX/newDY
			// (unit vectors via invLen). We store these normalized values in
			// nodeState.LastEdgeDX/DY so the dot product cosAngle gives [-1,1].
			turnCost, newDX, newDY := computeTurnCost(
				curLon, curLat, ev.TargetLon, ev.TargetLat,
				ns.LastEdgeDX, ns.LastEdgeDY, cfg.TurnCost, ev.Dist)

			edgeCost := distCost + elevCost + initialCost + turnCost
			newCost := ns.BestCost + edgeCost

			// elevationCorrection: latent buffer costs for closed-set comparison
			newEffective := newCost + newPathState.elevationCorrection(cfg)

			// Absolute pruning against maxCost (pass1 only)
			if maxCost > 0 && newEffective > maxCost {
				return
			}

			// Heuristic: air distance to destination
			h := equirectDist(ev.TargetLat, ev.TargetLon, dstLat, dstLon)
			fCost := newEffective + passCoeff*h

			// Dominance check: only update if new effective cost is strictly better
			tgtNS, exists := state.nodes[ev.Target]
			if !exists || newEffective < tgtNS.EffectiveCost {
				state.nodes[ev.Target] = nodeState{
					BestCost:      newCost,
					EffectiveCost: newEffective,
					FrontierCost:  fCost,              // stale-entry guard: f(n) = g(n) + passCoeff*h(n)
					Parent:        entry.NodeID,
					EdgeHandle:    ev.ElevHandle,
					EdgeTags1:     ev.Tags1,          // A09: store Tags1 for O(1) retrack lookup
					PathState:     newPathState,       // K14: consistent naming
					LastEdgeDX:    newDX,              // R01: normalized from computeTurnCost
					LastEdgeDY:    newDY,              // R01: normalized from computeTurnCost
					LastWayClass:  ecf.WayClass,
				}
				frontier.Push(FrontierEntry{NodeID: ev.Target, Cost: fCost})
			}
		})
	}

	return nil, fmt.Errorf("no route found (expanded %d nodes)", expansions)
}

// buildTrackIndexUnified creates a mapping from pass0 path nodes to remaining cost.
// remainingCost = totalCost - costAtNode (clamped to >= 0).
// Used by pass1 for dynamic bound-tightening (C02).
func buildTrackIndexUnified(state *SparseRouterState, src, dst uint64) map[uint64]float32 {
	// Walk parent chain from dst -> src
	path := []uint64{dst}
	for path[len(path)-1] != src {
		ns, ok := state.nodes[path[len(path)-1]]
		if !ok {
			return nil // broken chain
		}
		path = append(path, ns.Parent)
		if len(path) > 1_000_000 {
			return nil // safety limit
		}
	}

	totalCost := state.nodes[dst].BestCost
	trackIndex := make(map[uint64]float32, len(path))
	for _, nodeID := range path {
		ns := state.nodes[nodeID]
		remaining := totalCost - ns.BestCost
		if remaining < 0 {
			remaining = 0
		}
		trackIndex[nodeID] = remaining
	}
	return trackIndex
}

// retrackUnified reconstructs the route path with full geometry, elevation,
// distance, ascent/descent, and RouteSegments for the frontend.
//
// Geometry: Node coordinates have real elevation; DP-simplified intermediate
// points use Ele=-9999 sentinel (K08) because DP survivors lose their original
// elevation. The frontend uses a separate elevation profile for height charts.
//
// Segments: Contiguous edges with the same OSM way ID are grouped into
// RouteSegments with highway/surface/sac_scale/mtb_scale/tracktype metadata.
func retrackUnified(reader GraphReader, state *SparseRouterState,
	src, dst uint64) *RouteResult {

	// Walk parent chain from dst -> src
	type pathEdge struct {
		nodeID     uint64
		edgeHandle uint64
	}

	var chain []pathEdge
	node := dst
	maxIter := 1_000_000
	for node != src && maxIter > 0 {
		ns, ok := state.nodes[node]
		if !ok {
			return nil // broken chain
		}
		chain = append(chain, pathEdge{nodeID: node, edgeHandle: ns.EdgeHandle})
		node = ns.Parent
		maxIter--
	}
	if maxIter == 0 {
		return nil // safety: infinite loop guard
	}

	// chain is dst->...->src-adjacent; add src and reverse
	chain = append(chain, pathEdge{nodeID: src})
	slices.Reverse(chain)
	// Now chain[0] = src, chain[len-1] = dst
	// chain[i+1].edgeHandle = edge from chain[i] to chain[i+1]

	nEdges := len(chain) - 1
	if nEdges <= 0 {
		// src == dst: single-point route
		lat, lon, ele := reader.NodeCoord(src)
		return &RouteResult{
			Coordinates: []LatLonEle{{
				Lat: float64(lat) / 1e7,
				Lon: float64(lon) / 1e7,
				Ele: float64(ele) / 10.0,
			}},
			Cost: float64(state.nodes[dst].BestCost),
		}
	}

	// Build coordinates with edge geometry expansion
	var coords []LatLonEle
	var totalAscent, totalDescent float64

	// edgeCoordStart[i] = coordinate index where edge i starts (the network node)
	edgeCoordStart := make([]int, nEdges)
	// edgeTags1[i] = Tags1 for edge i (used for segment metadata)
	edgeTags1 := make([]uint64, nEdges)

	for i := range nEdges {
		// Record the edge handle and tags from the next node's state
		edgeHandle := chain[i+1].edgeHandle

		// Add the network node itself (start of this edge)
		nodeLat, nodeLon, nodeEle := reader.NodeCoord(chain[i].nodeID)
		edgeCoordStart[i] = len(coords)
		coords = append(coords, LatLonEle{
			Lat: float64(nodeLat) / 1e7,
			Lon: float64(nodeLon) / 1e7,
			Ele: float64(nodeEle) / 10.0, // dm -> m
		})

		// DP-simplified intermediate geometry for route display
		// K08: Ele=-9999 sentinel for DP points (no elevation available)
		// A04: EdgeGeometry returns ONLY intermediate points (start/end excluded by reader)
		lats, lons := reader.EdgeGeometry(edgeHandle)
		for j := range lats {
			coords = append(coords, LatLonEle{
				Lat: float64(lats[j]) / 1e7,
				Lon: float64(lons[j]) / 1e7,
				Ele: -9999, // K08: sentinel — DP points have no elevation
			})
		}

		// Full elevation profile for ascent/descent computation
		elevs, _ := reader.EdgeElevProfile(edgeHandle)
		for j := 1; j < len(elevs); j++ {
			diff := float64(elevs[j]-elevs[j-1]) / 10.0 // dm -> m
			if diff > 0 {
				totalAscent += diff
			} else {
				totalDescent -= diff
			}
		}

		// A09: Tags1 for segment metadata — stored in nodeState during A*, O(1) lookup.
		edgeTags1[i] = state.nodes[chain[i+1].nodeID].EdgeTags1
	}

	// Add last node (destination)
	dstLat, dstLon, dstEle := reader.NodeCoord(dst)
	coords = append(coords, LatLonEle{
		Lat: float64(dstLat) / 1e7,
		Lon: float64(dstLon) / 1e7,
		Ele: float64(dstEle) / 10.0,
	})

	// K09: Calculate total distance from coordinates using haversineDist (consistent with result.go)
	var totalDist float64
	for i := 1; i < len(coords); i++ {
		totalDist += haversineDist(coords[i-1].Lat, coords[i-1].Lon, coords[i].Lat, coords[i].Lon)
	}

	// Build RouteSegments: group contiguous edges with same Tags1
	segments := buildSegmentsUnified(nEdges, edgeCoordStart,
		edgeTags1, len(coords)-1)

	return &RouteResult{
		Coordinates: coords,
		DistanceM:   totalDist,
		AscentM:     totalAscent,
		DescentM:    totalDescent,
		Cost:        float64(state.nodes[dst].BestCost),
		Segments:    segments,
	}
}

// buildSegmentsUnified groups edges into RouteSegments by Tags1 identity.
// Tags1 encodes highway, surface, tracktype, sac_scale, mtb_scale — changes in
// any of these start a new segment. This mirrors the old makeSegment/OSMIdx logic
// but without requiring EdgeOSMIdx (which is graph-local, not available via GraphReader).
func buildSegmentsUnified(nEdges int,
	edgeCoordStart []int, edgeTags1 []uint64,
	lastCoordIdx int) []RouteSegment {

	if nEdges == 0 {
		return nil
	}

	var segments []RouteSegment
	curTags := edgeTags1[0]
	segFromCoord := edgeCoordStart[0]

	for i := 1; i < nEdges; i++ {
		if edgeTags1[i] != curTags {
			segments = append(segments, makeSegmentFromTags(segFromCoord, edgeCoordStart[i], curTags))
			segFromCoord = edgeCoordStart[i]
			curTags = edgeTags1[i]
		}
	}
	// Final segment
	segments = append(segments, makeSegmentFromTags(segFromCoord, lastCoordIdx, curTags))

	return segments
}

// makeSegmentFromTags creates a RouteSegment from tag-encoded metadata.
func makeSegmentFromTags(fromIdx, toIdx int, tags uint64) RouteSegment {
	return RouteSegment{
		FromIdx:   fromIdx,
		ToIdx:     toIdx,
		Highway:   graph.HighwayName(graph.DecodeHighway(tags)),
		Surface:   graph.SurfaceName(graph.DecodeSurface(tags)),
		SacScale:  graph.SacScaleName(uint8(graph.DecodeField(tags, 14, 3))),
		MtbScale:  graph.MtbScaleName(uint8(graph.DecodeField(tags, 17, 3))),
		Tracktype: graph.TrackTypeName(uint8(graph.DecodeField(tags, 11, 3))),
		OsmWayID:  0, // Not available via GraphReader — filled by TiledGraphReader if needed
	}
}
