// unified.go — Unified A* routing with GraphReader interface.
// Replaces: pass0, pass1, pass0Sparse, pass1Sparse, retrack, retrackSparse.
// One findRouteUnified handles both pass0 (inadmissible A*, c>1) and pass1 (Dijkstra, c=0).
// Spec: §9.1-9.5
package routing

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// noParent is the sentinel value for nodeState.Parent indicating no predecessor
// (seed nodes and unrelaxed virtual endpoint states).
// MaxUint64 is used because NodeID 0 is a valid graph node.
const noParent = ^uint64(0)

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
	Expanded      bool      // Closed-set flag: prevents re-relaxation after expansion
}

// SparseRouterState uses a value-type map for unknown node counts.
// C19: map[uint64]nodeState (value-type, not pointer) — eliminates per-entry
// heap allocation and GC pressure.
type SparseRouterState struct {
	nodes    map[uint64]nodeState
	srcSeedA uint64 // source seed node A (for retrack termination)
	srcSeedB uint64 // source seed node B (same as A for legacy)
}

func newSparseRouterState(hint int) *SparseRouterState {
	return &SparseRouterState{nodes: make(map[uint64]nodeState, hint)}
}

// routeResult is the internal result of a single findRouteUnified call.
type routeResult struct {
	state       *SparseRouterState
	totalCost   float32
	expansions  int
	durationMs  int64
	bestDstNode uint64 // which destination node was reached (for VE retrack)
}

const (
	unifiedMaxExpansions = 10_000_000
	unifiedCheckCancel   = 4096
)

// findRouteUnified — unified A* for pass0 and pass1.
//
// passCoeff > 1.0 for pass0 (inadmissible A*), 0 for pass1 (Dijkstra).
// trackIndex: nil for pass0, map from pass0 path for pass1 cost-cutting (R06).
// maxCost: 0 for pass0 (unlimited), pass0 totalCost for pass1.
// landmarkH: optional ALT heuristic (nil = euclidean only).
//
// Returns routeResult with state, totalCost, expansions, and duration.
func findRouteUnified(ctx context.Context, reader GraphReader,
	srcVE *VirtualEndpoint, dstVE *VirtualEndpoint,
	srcNode, dstNode uint64, // used only when corresponding VE is nil
	cfg *CostConfig,
	passCoeff float32, maxCost float32,
	trackIndex map[uint64]float32,
	landmarkH *LandmarkHeuristic, // nil = nur euklidische Heuristik
	bwd *BackwardResult, // nil = kein Pruning
) (*routeResult, error) {

	start := time.Now()

	var srcLat, srcLon int32
	var dstLat, dstLon, dstEle int32

	// Determine effective destination nodes
	var effectiveDstA, effectiveDstB uint64
	if dstVE != nil {
		effectiveDstA = dstVE.GlobalA
		effectiveDstB = dstVE.GlobalB
		dstLat = int32(dstVE.OrigLat * 1e7)
		dstLon = int32(dstVE.OrigLon * 1e7)
		_, _, dstEle = reader.NodeCoord(effectiveDstA)
	} else {
		effectiveDstA = dstNode
		effectiveDstB = dstNode
		dstLat, dstLon, dstEle = reader.NodeCoord(dstNode)
	}

	if srcVE != nil {
		srcLat = int32(srcVE.OrigLat * 1e7)
		srcLon = int32(srcVE.OrigLon * 1e7)
	} else {
		srcLat, srcLon, _ = reader.NodeCoord(srcNode)
	}

	// Pre-size based on euclidean distance: ~30,000 nodes per km of air distance.
	// Eliminates ~10 rehash cycles for long routes (e.g. 2.6M expansions with 8192 hint).
	euclidKm := equirectDistKm(srcLat, srcLon, dstLat, dstLon)
	stateHint := max(8192, int(euclidKm*30_000))
	state := newSparseRouterState(stateHint)
	frontier := Frontier{entries: make([]FrontierEntry, 0, 8192)} // K17: capacity hint

	// Seed source
	if srcVE != nil {
		state.nodes[srcVE.GlobalA] = srcVE.StateA
		frontier.Push(FrontierEntry{NodeID: srcVE.GlobalA, Cost: srcVE.StateA.EffectiveCost})
		state.nodes[srcVE.GlobalB] = srcVE.StateB
		frontier.Push(FrontierEntry{NodeID: srcVE.GlobalB, Cost: srcVE.StateB.EffectiveCost})
		state.srcSeedA = srcVE.GlobalA
		state.srcSeedB = srcVE.GlobalB
	} else {
		state.nodes[srcNode] = nodeState{BestCost: 0, EffectiveCost: 0, FrontierCost: 0, Parent: noParent}
		frontier.Push(FrontierEntry{NodeID: srcNode, Cost: 0})
		state.srcSeedA = srcNode
		state.srcSeedB = srcNode
	}

	// h_elev pre-computation: min(UphillCost, DownhillCost) for elevation heuristic.
	// For 3 of 5 profiles this is 0 (no elevation heuristic, no overhead).
	// Capped at 10 to prevent overly aggressive pruning with high elevation costs
	// (e.g. Gravel minElevCost=40 would make h_elev dominate and slow Pass0).
	minElevCost := min(cfg.UphillCost, cfg.DownhillCost, 10)

	var bestDstCost float32 = math.MaxFloat32
	var bestDstNode uint64
	expansions := 0

	for frontier.Len() > 0 {
		// Periodic context cancellation check
		if expansions&(unifiedCheckCancel-1) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("%w (after %d expansions, %dms)",
					err, expansions, time.Since(start).Milliseconds())
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

		// Mark node as expanded (closed-set). Prevents parent-pointer cycles
		// caused by hysteresis re-relaxation with high elevation costs.
		ns.Expanded = true
		state.nodes[entry.NodeID] = ns

		// Dual destination termination
		if dstVE != nil {
			if entry.NodeID == effectiveDstA || entry.NodeID == effectiveDstB {
				totalCost := ns.EffectiveCost
				if entry.NodeID == effectiveDstA {
					totalCost += dstVE.CostA
				} else {
					totalCost += dstVE.CostB
				}
				// Use <= (not <) so that a non-seed destination with equal cost
				// replaces a seed destination. This prevents the bestDstNode from
				// being stuck on an unrelaxed seed when src/dst VEs share a node.
				if totalCost <= bestDstCost {
					bestDstCost = totalCost
					bestDstNode = entry.NodeID
				}
			}

			if passCoeff > 0 {
				// Pass0 (inadmissible): terminate once any destination is reached and expanded.
				// Both-expanded is unnecessary — inadmissible A* provides no optimality guarantee.
				if bestDstCost < math.MaxFloat32 {
					dstAExpanded := false
					dstBExpanded := false
					if nsA, ok := state.nodes[effectiveDstA]; ok {
						dstAExpanded = nsA.Expanded
					}
					if nsB, ok := state.nodes[effectiveDstB]; ok {
						dstBExpanded = nsB.Expanded
					}
					if dstAExpanded || dstBExpanded {
						return &routeResult{
							state: state, totalCost: bestDstCost,
							expansions: expansions, durationMs: time.Since(start).Milliseconds(),
							bestDstNode: bestDstNode,
						}, nil
					}
				}
			}

			// Pass1 Dijkstra termination: if min frontier cost exceeds best
			// known destination cost, no remaining path can improve. However,
			// skip termination when the bestDstNode is an unrelaxed source seed
			// — its cost comes from partial edge seeding, not real edge traversal.
			// Terminating early in this case would prevent discovery of better
			// paths through the other seed node (shared src/dst VE node).
			bestIsUnrelaxedSeed := (bestDstNode == state.srcSeedA || bestDstNode == state.srcSeedB) &&
				state.nodes[bestDstNode].Parent == noParent
			if passCoeff == 0 && bestDstCost < math.MaxFloat32 && !bestIsUnrelaxedSeed &&
				frontier.PeekCost() > bestDstCost {
				return &routeResult{
					state: state, totalCost: bestDstCost,
					expansions: expansions, durationMs: time.Since(start).Milliseconds(),
					bestDstNode: bestDstNode,
				}, nil
			}
		} else {
			// Legacy single destination
			if entry.NodeID == effectiveDstA {
				return &routeResult{
					state: state, totalCost: ns.EffectiveCost,
					expansions: expansions, durationMs: time.Since(start).Milliseconds(),
					bestDstNode: effectiveDstA,
				}, nil
			}
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

		// Backward pruning (bidirectional, pass0 only)
		if bwd != nil && bwd.ShouldPrune(entry.NodeID, ns.EffectiveCost) {
			continue
		}

		// Current node coordinates (once per expansion, for turn cost direction)
		curLat, curLon, curEle := reader.NodeCoord(entry.NodeID)

		reader.ForEachEdge(entry.NodeID, func(ev EdgeView) {
			ecf := cfg.EdgeCost(ev.Tags1, ev.Tags2)
			if ecf.Flat >= costBlocked {
				return // impassable
			}

			// Way penalty: bias repeated edges (round-trip quality gate, phase 1b.5).
			// Fast path: returns 1.0 immediately when WayPenalties is nil/empty.
			penalty := lookupWayPenalty(ev.ElevHandle, reader, cfg.WayPenalties)
			applyPenaltyToECF(&ecf, penalty)

			// Hysteresis: elevation cost + slope weights
			var newPathState PathState
			var elevCost, upWeight, downWeight float32

			if passCoeff > 1.0 {
				// Pass0 (inadmissible, c>1.0): DeltaEle for speed.
				// DeltaEle underestimates on hilly terrain (net vs actual elevation work),
				// but pass1MaxCost compensates with squared Pass0Coefficient margin.
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

			// Apply penalty to elevation cost as well (penalty > 1.0 check is free).
			if penalty > 1.0 {
				elevCost *= penalty
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

			// Heuristic: max(euclidean, ALT, elevation)
			hEuclid := equirectDist(ev.TargetLat, ev.TargetLon, dstLat, dstLon)
			var h float32
			if landmarkH != nil {
				hALT := landmarkH.LowerBound(ev.Target)
				h = max(hEuclid, hALT)
			} else {
				h = hEuclid
			}

			// h_elev: admissible elevation-based lower bound.
			// targetEle = curEle + DeltaEle (both in dm, DeltaEle = target - source).
			// For profiles where min(UphillCost, DownhillCost) = 0, this is a no-op.
			if minElevCost > 0 {
				targetEle := curEle + ev.DeltaEle
				elevDiffDm := targetEle - dstEle
				if elevDiffDm < 0 {
					elevDiffDm = -elevDiffDm
				}
				elevDiffM := float32(elevDiffDm) / 10.0 // dm -> m
				hElev := max(0, elevDiffM-cfg.ElevPenaltyBuffer) * minElevCost
				h = max(h, hElev)
			}

			fCost := newEffective + passCoeff*h

			// Dominance check: only update if new effective cost is strictly better.
			// pass1 (passCoeff=0, Dijkstra): also respect Expanded flag — once a node
			// is in the closed set it cannot be re-relaxed (prevents parent-pointer cycles
			// caused by hysteresis re-relaxation with high elevation costs, e.g. Gravel).
			// pass0 (passCoeff>0, inadmissible A*): always allow re-relaxation.
			tgtNS, exists := state.nodes[ev.Target]
			if !exists || (newEffective < tgtNS.EffectiveCost && (passCoeff > 0 || !tgtNS.Expanded)) {
				state.nodes[ev.Target] = nodeState{
					BestCost:      newCost,
					EffectiveCost: newEffective,
					FrontierCost:  fCost, // stale-entry guard: f(n) = g(n) + passCoeff*h(n)
					Parent:        entry.NodeID,
					EdgeHandle:    ev.ElevHandle,
					EdgeTags1:     ev.Tags1,     // A09: store Tags1 for O(1) retrack lookup
					PathState:     newPathState, // K14: consistent naming
					LastEdgeDX:    newDX,        // R01: normalized from computeTurnCost
					LastEdgeDY:    newDY,        // R01: normalized from computeTurnCost
					LastWayClass:  ecf.WayClass,
				}
				frontier.Push(FrontierEntry{NodeID: ev.Target, Cost: fCost})
			}
		})
	}

	return nil, ErrNoRoute
}

// buildTrackIndexUnified creates a mapping from pass0 path nodes to remaining cost.
// remainingCost = totalCost - costAtNode (clamped to >= 0).
// Used by pass1 for dynamic bound-tightening (C02).
func buildTrackIndexUnified(state *SparseRouterState, dst uint64, stopAt uint64) map[uint64]float32 {
	// Walk parent chain from dst -> source seed (or stopAt)
	path := []uint64{dst}

	isTerminal := func(nodeID uint64) bool {
		if stopAt != 0 {
			return nodeID == stopAt
		}
		return nodeID == state.srcSeedA || nodeID == state.srcSeedB
	}

	for !isTerminal(path[len(path)-1]) {
		ns, ok := state.nodes[path[len(path)-1]]
		if !ok {
			return nil // broken chain
		}
		path = append(path, ns.Parent)
		if len(path) > 1_000_000 {
			return nil // safety limit
		}
	}

	// NOTE: Uses BestCost (raw g-cost) for remaining calculation, NOT EffectiveCost.
	// This is intentional: the trackIndex remaining values are scaled by Pass0Coefficient
	// in the cutoff formula (line ~126), providing a safety margin that compensates for
	// the BestCost→EffectiveCost gap (elevation correction). Using EffectiveCost here
	// would double-count the correction.
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
	bestDstNode uint64, srcVE *VirtualEndpoint, dstVE *VirtualEndpoint,
	cfg *CostConfig) *RouteResult {

	// Walk parent chain from bestDstNode -> src seed
	type pathEdge struct {
		nodeID     uint64
		edgeHandle uint64
	}

	var chain []pathEdge
	node := bestDstNode
	maxIter := 1_000_000
	for node != state.srcSeedA && node != state.srcSeedB && maxIter > 0 {
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
	// node is now the actual seed (srcSeedA or srcSeedB)
	chain = append(chain, pathEdge{nodeID: node})
	slices.Reverse(chain)
	// Now chain[0] = seed, chain[len-1] = bestDstNode
	// chain[i+1].edgeHandle = edge from chain[i] to chain[i+1]

	nEdges := len(chain) - 1
	if nEdges <= 0 {
		// single-point route
		lat, lon, ele := reader.NodeCoord(chain[0].nodeID)
		return &RouteResult{
			Coordinates: []LatLonEle{{
				Lat: float64(lat) / 1e7,
				Lon: float64(lon) / 1e7,
				Ele: float64(ele) / 10.0,
			}},
			Cost: float64(state.nodes[bestDstNode].BestCost),
		}
	}

	// Build coordinates with edge geometry expansion
	var coords []LatLonEle
	var totalAscent, totalDescent float64

	// Accumulators for RouteResult quality-gate metrics (phase 1b.6).
	var distSum, qualitySum, ascentSum, descentSum float32

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
		// A04: EdgeGeometry returns ONLY intermediate points (start/end excluded by reader)
		// Carry forward the network node's elevation for DP points (no per-point DEM available)
		lats, lons := reader.EdgeGeometry(edgeHandle)
		carriedEle := float64(nodeEle) / 10.0 // use this edge's start node elevation
		for j := range lats {
			coords = append(coords, LatLonEle{
				Lat: float64(lats[j]) / 1e7,
				Lon: float64(lons[j]) / 1e7,
				Ele: carriedEle,
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

		// Phase 1b.6: populate DistSum/QualitySum/AscentSum/DescentSum accumulators.
		// Use EdgeCostRaw (unpenalized) so QualitySum reflects true path quality.
		// Tags2 is not stored in nodeState, so we look it up via ForEachEdge by handle.
		if cfg != nil && cfg.EdgeCostRaw != nil {
			srcNode := chain[i].nodeID
			reader.ForEachEdge(srcNode, func(ev EdgeView) {
				if ev.ElevHandle != edgeHandle {
					return
				}
				ecfRaw := cfg.EdgeCostRaw(ev.Tags1, ev.Tags2)
				edgeDistM := float32(ev.Dist) / 100.0

				distSum += edgeDistM
				qualitySum += edgeDistM * ecfRaw.Flat

				if ev.DeltaEle > 0 {
					ascentSum += float32(ev.DeltaEle) / 10.0
				} else if ev.DeltaEle < 0 {
					descentSum += float32(-ev.DeltaEle) / 10.0
				}
			})
		}
	}

	// Add last node (destination)
	dstLat, dstLon, dstEle := reader.NodeCoord(bestDstNode)
	coords = append(coords, LatLonEle{
		Lat: float64(dstLat) / 1e7,
		Lon: float64(dstLon) / 1e7,
		Ele: float64(dstEle) / 10.0,
	})

	// Build RouteSegments: group contiguous edges with same Tags1
	segments := buildSegmentsUnified(nEdges, edgeCoordStart,
		edgeTags1, len(coords)-1)

	// Source-side: insert partial edge geometry from projection point to seed node.
	// Without this, there's a straight line (Luftlinie) from the projection to the
	// first network node — visually cutting across roads/bridges/water.
	if srcVE != nil {
		partialCoords := extractPartialEdgeGeometry(reader, srcVE, chain[0].nodeID, true)
		if len(partialCoords) > 0 {
			coords = append(partialCoords, coords...)
			for i := range segments {
				segments[i].FromIdx += len(partialCoords)
				segments[i].ToIdx += len(partialCoords)
			}
		}
	}

	// Destination-side: insert partial edge geometry from last node to projection point.
	if dstVE != nil {
		partialCoords := extractPartialEdgeGeometry(reader, dstVE, bestDstNode, false)
		coords = append(coords, partialCoords...)
	}

	// K09: Calculate total distance from coordinates using geo.DistanceHaversine
	// (after VE prepend/append so projected endpoints are included)
	var totalDist float64
	for i := 1; i < len(coords); i++ {
		totalDist += geo.DistanceHaversine(coords[i-1].Lat, coords[i-1].Lon, coords[i].Lat, coords[i].Lon)
	}

	return &RouteResult{
		Coordinates: coords,
		DistanceM:   totalDist,
		AscentM:     totalAscent,
		DescentM:    totalDescent,
		Cost:        float64(state.nodes[bestDstNode].BestCost),
		Segments:    segments,
		DistSum:     distSum,
		QualitySum:  qualitySum,
		AscentSum:   ascentSum,
		DescentSum:  descentSum,
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

// extractPartialEdgeGeometry builds the coordinate sequence for the partial edge
// between a VirtualEndpoint's projection point and the seed node that A* used.
//
// For source-side (isSource=true): returns [projPoint, ...intermediates..., seedNode].
// The seed node is where the VE connects to the A* graph (GlobalA or GlobalB).
// The route already starts at the seed node, so this partial geometry fills the
// gap between the projection point and the seed — following the actual road geometry
// instead of a straight line (Luftlinie).
//
// For destination-side (isSource=false): returns [seedNode, ...intermediates..., projPoint].
// Note: the seed node coordinate is NOT included in the output (it's already the last
// coord from the main retrack loop). Only intermediate + projection point are returned.
func extractPartialEdgeGeometry(reader GraphReader, ve *VirtualEndpoint,
	seedNode uint64, isSource bool) []LatLonEle {

	proj := ve.Proj

	// Build full edge polyline: [NodeA, ...intermediates..., NodeB]
	nodeALat, nodeALon, nodeAEle := reader.NodeCoord(ve.GlobalA)
	nodeBLat, nodeBLon, nodeBEle := reader.NodeCoord(ve.GlobalB)
	intermLats, intermLons := reader.EdgeGeometry(proj.EdgeHandle)

	type pt struct{ lat, lon, ele float64 }
	full := make([]pt, 0, len(intermLats)+2)
	full = append(full, pt{float64(nodeALat) / 1e7, float64(nodeALon) / 1e7, float64(nodeAEle) / 10.0})
	for i := range intermLats {
		// Intermediate points don't have elevation — interpolate linearly
		frac := float64(i+1) / float64(len(intermLats)+1)
		ele := float64(nodeAEle)/10.0 + frac*(float64(nodeBEle)/10.0-float64(nodeAEle)/10.0)
		full = append(full, pt{float64(intermLats[i]) / 1e7, float64(intermLons[i]) / 1e7, ele})
	}
	full = append(full, pt{float64(nodeBLat) / 1e7, float64(nodeBLon) / 1e7, float64(nodeBEle) / 10.0})

	if len(full) < 2 {
		// Degenerate edge — just return the projection point
		return []LatLonEle{{
			Lat: proj.ProjLat, Lon: proj.ProjLon,
			Ele: float64(proj.ProjEleDm) / 10.0,
		}}
	}

	// Compute cumulative distances to find where FractionAB falls
	cumDist := make([]float64, len(full))
	for i := 1; i < len(full); i++ {
		cumDist[i] = cumDist[i-1] + geo.DistanceHaversine(full[i-1].lat, full[i-1].lon, full[i].lat, full[i].lon)
	}
	totalLen := cumDist[len(cumDist)-1]
	if totalLen == 0 {
		return []LatLonEle{{Lat: proj.ProjLat, Lon: proj.ProjLon, Ele: float64(proj.ProjEleDm) / 10.0}}
	}

	projDist := proj.FractionAB * totalLen // distance from NodeA to projection point

	// Find which segment of the polyline the projection falls on
	projSegIdx := 0
	for i := 1; i < len(cumDist); i++ {
		if cumDist[i] >= projDist {
			projSegIdx = i - 1
			break
		}
	}

	projPt := pt{proj.ProjLat, proj.ProjLon, float64(proj.ProjEleDm) / 10.0}

	// Determine which end of the edge the seed node is on
	seedIsA := seedNode == ve.GlobalA

	var result []LatLonEle
	if isSource {
		// Source side: projection → seed node direction
		// We need points FROM projPt TO the seed.
		if seedIsA {
			// Proj → NodeA: go backwards along the polyline
			// Points: projPt, full[projSegIdx], full[projSegIdx-1], ..., full[0]=NodeA
			result = append(result, LatLonEle{Lat: projPt.lat, Lon: projPt.lon, Ele: projPt.ele})
			for i := projSegIdx; i >= 0; i-- {
				result = append(result, LatLonEle{Lat: full[i].lat, Lon: full[i].lon, Ele: full[i].ele})
			}
		} else {
			// Proj → NodeB: go forwards along the polyline
			// Points: projPt, full[projSegIdx+1], ..., full[last]=NodeB
			result = append(result, LatLonEle{Lat: projPt.lat, Lon: projPt.lon, Ele: projPt.ele})
			for i := projSegIdx + 1; i < len(full); i++ {
				result = append(result, LatLonEle{Lat: full[i].lat, Lon: full[i].lon, Ele: full[i].ele})
			}
		}
		// Remove the last point (seed node) — it's already in the main retrack chain
		if len(result) > 1 {
			result = result[:len(result)-1]
		}
	} else {
		// Destination side: seed node → projection point
		// Seed node is already the last coord from retrack. We only add intermediates + proj.
		if seedIsA {
			// NodeA → Proj: go forwards
			for i := 1; i <= projSegIdx; i++ {
				result = append(result, LatLonEle{Lat: full[i].lat, Lon: full[i].lon, Ele: full[i].ele})
			}
			result = append(result, LatLonEle{Lat: projPt.lat, Lon: projPt.lon, Ele: projPt.ele})
		} else {
			// NodeB → Proj: go backwards
			for i := len(full) - 2; i > projSegIdx; i-- {
				result = append(result, LatLonEle{Lat: full[i].lat, Lon: full[i].lon, Ele: full[i].ele})
			}
			result = append(result, LatLonEle{Lat: projPt.lat, Lon: projPt.lon, Ele: projPt.ele})
		}
	}

	return result
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
