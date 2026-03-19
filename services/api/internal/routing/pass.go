// pass.go
package routing

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// computeTurnCost returns the turn cost and the normalized direction of the new edge.
// lastDX/lastDY are the previous edge direction (0,0 if no previous edge).
// turnCostBase is cfg.TurnCost (0 means no turn costs).
// edgeDistCm is the edge distance in centimeters — used to scale turn cost proportionally
// so short edges (< 200m, common after TNE in urban areas) don't accumulate excessive turn costs.
func computeTurnCost(curLon, curLat, targetLon, targetLat int32,
	lastDX, lastDY, turnCostBase float32, edgeDistCm uint32) (turnCost, newDX, newDY float32) {

	edgeDX := float32(targetLon - curLon)
	edgeDY := float32(targetLat - curLat)
	lenSq := edgeDX*edgeDX + edgeDY*edgeDY
	if lenSq > 0 {
		invLen := 1.0 / float32(math.Sqrt(float64(lenSq)))
		newDX = edgeDX * invLen
		newDY = edgeDY * invLen
	}

	if turnCostBase > 0 && lastDX != 0 && lenSq > 0 {
		cosAngle := lastDX*newDX + lastDY*newDY
		// Scale turn cost by edge distance: short edges (< 200m) get proportionally
		// less turn cost. Without this, TNE edges in pedestrian zones (2-50m) accumulate
		// 60+ turn costs that blow the pass0 budget for 2km routes.
		distScale := min(1.0, float32(edgeDistCm)/20000.0) // 20000 cm = 200m
		turnCost = turnCostBase * (1.0 - cosAngle) * distScale
	}
	return
}

// computeEdgeCostWeighted computes the weighted edge cost from costfactors and hysteresis weights.
func computeEdgeCostWeighted(ecf EdgeCostFactors, distM float32, hr HysteresisResult,
	turnCost float32, curLastWayClass uint8) float32 {

	cfUp := ecf.Up
	cfDown := ecf.Down
	if cfUp == 0 {
		cfUp = ecf.Flat
	}
	if cfDown == 0 {
		cfDown = ecf.Flat
	}
	flatW := 1.0 - hr.UpWeight - hr.DownWeight
	if flatW < 0 {
		flatW = 0
	}
	costfactor := cfUp*hr.UpWeight + ecf.Flat*flatW + cfDown*hr.DownWeight
	edgeCost := turnCost + hr.ElevCost + distM*costfactor

	// Initialcost (way class change)
	if curLastWayClass != ecf.WayClass && curLastWayClass != 0 {
		if ecf.WayClass == 1 || curLastWayClass == 1 { // ferry transition
			edgeCost += 10000 // BRouter: 10km equivalent
		}
	}
	return edgeCost
}

const checkCancelEvery = 4096

// maxExpansions limits the number of node expansions per pass to prevent
// unbounded memory growth on large graphs without WCC pruning.
// 2M expansions covers 200+ km routes on a 25M-node graph.
const maxExpansions = 2_000_000

// edgeSegmentDistances computes distances in millimeters between consecutive
// geometry points for an edge.
func edgeSegmentDistances(g *graph.Graph, ei uint32) []int32 {
	if g.EdgeGeomLat == nil {
		return nil
	}
	profStart := g.EdgeElevOff[ei]
	profEnd := g.EdgeElevOff[ei+1]
	nPoints := int(profEnd - profStart)
	if nPoints < 2 {
		return nil
	}
	geomStart := g.EdgeGeomOff[ei]
	dists := make([]int32, nPoints-1)
	for j := range nPoints - 1 {
		lat1 := g.EdgeGeomLat[geomStart+uint32(j)]
		lon1 := g.EdgeGeomLon[geomStart+uint32(j)]
		lat2 := g.EdgeGeomLat[geomStart+uint32(j)+1]
		lon2 := g.EdgeGeomLon[geomStart+uint32(j)+1]
		distM := equirectDist(lat1, lon1, lat2, lon2)
		dists[j] = int32(distM * 1000) // meters → millimeters
	}
	return dists
}

// pass0 runs an inadmissible A* (coefficient > 1) to find a fast upper bound.
// Returns the cost-cutting track (node IDs along the found path) and maxTotalCost.
func pass0(ctx context.Context, g *graph.Graph, state *RouterState,
	src, dst uint32, cfg *CostConfig) ([]uint32, float32, *PassStats, error) {

	start := time.Now()
	state.frontier.Reset()

	dstLat := g.NodeLat[dst]
	dstLon := g.NodeLon[dst]

	airDist := float32(cfg.Pass0Coefficient) * equirectDist(g.NodeLat[src], g.NodeLon[src], dstLat, dstLon)
	state.frontier.Push(FrontierEntry{
		NodeID:  src,
		Cost:    0,
		AirDist: airDist,
		Parent:  src,
	})
	state.bestCost[src] = 0
	state.nodeGen[src] = state.generation

	expansions := 0

	for state.frontier.Len() > 0 {
		if expansions&(checkCancelEvery-1) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, nil, err
			}
		}
		expansions++
		if expansions > maxExpansions {
			return nil, 0, nil, fmt.Errorf("pass0: expansion limit reached (%d), route too long or graph disconnected", maxExpansions)
		}

		cur := state.frontier.Pop()

		// Skip stale entries (must come before destination check in pass0,
		// because inadmissible A* can revisit nodes with lower cost).
		// Note: cur.Cost is RAW (no elevation correction), bestCost stores EFFECTIVE
		// (raw + elevationCorrection). We recompute effective here for comparison.
		if state.nodeGen[cur.NodeID] == state.generation &&
			cur.Cost+cur.State.elevationCorrection(cfg) > state.bestCost[cur.NodeID] {
			continue
		}

		// Reached destination
		if cur.NodeID == dst {
			track := extractTrack(state, src, dst, g.NodeCount)
			stats := &PassStats{
				Pass0Expansions: expansions,
				Pass0DurationMs: time.Since(start).Milliseconds(),
			}
			// Return effective cost (not raw) — must match buildTrackIndex semantics
			effectiveCost := cur.Cost + cur.State.elevationCorrection(cfg)
			return track, effectiveCost, stats, nil
		}

		// Expand neighbors
		edgeStart := g.Offsets[cur.NodeID]
		edgeEnd := g.Offsets[cur.NodeID+1]
		for ei := edgeStart; ei < edgeEnd; ei++ {
			target := g.Targets[ei]

			ecf := cfg.EdgeCost(g, ei)
			if ecf.Flat >= costBlocked {
				continue
			}

			// Turncost + direction
			tc, newDX, newDY := computeTurnCost(
				g.NodeLon[cur.NodeID], g.NodeLat[cur.NodeID],
				g.NodeLon[target], g.NodeLat[target],
				cur.LastEdgeDX, cur.LastEdgeDY, cfg.TurnCost, g.EdgeDist[ei])

			// Hysteresis
			var hr HysteresisResult
			profStart := g.EdgeElevOff[ei]
			profEnd := g.EdgeElevOff[ei+1]
			if profEnd > profStart && profEnd-profStart <= 5000 {
				profile := g.EdgeElevProf[profStart:profEnd]
				segDist := edgeSegmentDistances(g, ei)
				hr = propagateHysteresisProfile(cur.State, profile, segDist, cfg)
			} else {
				deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100
				distMilli := int32(g.EdgeDist[ei]) * 10
				hr = propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			}

			// Slope-weighted costfactor interpolation
			distM := float32(g.EdgeDist[ei]) / 100.0
			edgeCost := computeEdgeCostWeighted(ecf, distM, hr, tc, cur.LastWayClass)
			newCost := cur.Cost + edgeCost
			effectiveCost := newCost + hr.State.elevationCorrection(cfg)

			// Pruning: bounded early-exit + dominance check
			if state.nodeGen[target] == state.generation {
				if newCost > state.bestCost[target]+cfg.MaxLatentCost {
					continue
				}
				if effectiveCost >= state.bestCost[target] {
					continue
				}
			}

			state.bestCost[target] = effectiveCost
			state.nodeGen[target] = state.generation
			state.parentNode[target] = cur.NodeID
			state.parentEdge[target] = ei

			airD := float32(cfg.Pass0Coefficient) * equirectDist(g.NodeLat[target], g.NodeLon[target], dstLat, dstLon)
			state.frontier.Push(FrontierEntry{
				NodeID:       target,
				Cost:         newCost,
				AirDist:      airD,
				State:        hr.State,
				Parent:       cur.NodeID,
				Edge:         ei,
				LastEdgeDX:   newDX,
				LastEdgeDY:   newDY,
				LastWayClass: ecf.WayClass,
			})
		}
	}

	return nil, 0, nil, fmt.Errorf("no route found (pass0: expanded %d nodes)", expansions)
}

// pass1 runs a Dijkstra (c=0) with adaptive cost-cutting from the pass0 track.
// trackIndex must be built BEFORE advanceGeneration (pass0 data is in old generation).
func pass1(ctx context.Context, g *graph.Graph, state *RouterState,
	src, dst uint32, cfg *CostConfig,
	trackIndex map[uint32]float32, maxTotalCost float32) (*PassStats, error) {

	start := time.Now()
	state.frontier.Reset()

	state.frontier.Push(FrontierEntry{
		NodeID: src,
		Cost:   0,
		Parent: src,
	})
	state.bestCost[src] = 0
	state.nodeGen[src] = state.generation

	expansions := 0

	for state.frontier.Len() > 0 {
		if expansions&(checkCancelEvery-1) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		expansions++
		if expansions > maxExpansions {
			return nil, fmt.Errorf("pass1: expansion limit reached (%d)", maxExpansions)
		}

		cur := state.frontier.Pop()

		// Compute effective cost (raw + latent hysteresis buffer).
		// Note: cur.Cost is RAW, bestCost stores EFFECTIVE.
		effectiveCur := cur.Cost + cur.State.elevationCorrection(cfg)

		// Skip stale entries BEFORE destination check and cost-cutting — ensures
		// only settled (non-superseded) entries tighten maxTotalCost, and prevents
		// returning a suboptimal destination entry superseded by a better path.
		if state.nodeGen[cur.NodeID] == state.generation &&
			effectiveCur > state.bestCost[cur.NodeID] {
			continue
		}

		// Reached destination (after stale check — guarantees optimal)
		if cur.NodeID == dst {
			return &PassStats{
				Pass1Expansions: expansions,
				Pass1DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// Cost-cutting: if pass1 reaches a node on the pass0 track with lower cost,
		// the predicted total (effectiveCur + remaining along pass0 track) tightens the bound.
		// SAFETY MARGIN: pass0 is inadmissible (c>1, typically 3.0 for trekking) so its track
		// costs are heavily distorted — remaining from pass0 can underestimate the actual
		// remaining cost by 15-20%. Use pass0Coefficient as margin to compensate.
		if remaining, ok := trackIndex[cur.NodeID]; ok {
			cutoff := effectiveCur + remaining*cfg.Pass0Coefficient
			if cutoff < maxTotalCost {
				maxTotalCost = cutoff
			}
		}

		// Prune by maxTotalCost
		if effectiveCur > maxTotalCost {
			continue
		}

		// Expand neighbors
		edgeStart := g.Offsets[cur.NodeID]
		edgeEnd := g.Offsets[cur.NodeID+1]
		for ei := edgeStart; ei < edgeEnd; ei++ {
			target := g.Targets[ei]

			ecf := cfg.EdgeCost(g, ei)
			if ecf.Flat >= costBlocked {
				continue
			}

			// Turncost + direction
			tc, newDX, newDY := computeTurnCost(
				g.NodeLon[cur.NodeID], g.NodeLat[cur.NodeID],
				g.NodeLon[target], g.NodeLat[target],
				cur.LastEdgeDX, cur.LastEdgeDY, cfg.TurnCost, g.EdgeDist[ei])

			// Hysteresis
			var hr HysteresisResult
			profStart := g.EdgeElevOff[ei]
			profEnd := g.EdgeElevOff[ei+1]
			if profEnd > profStart && profEnd-profStart <= 5000 {
				profile := g.EdgeElevProf[profStart:profEnd]
				segDist := edgeSegmentDistances(g, ei)
				hr = propagateHysteresisProfile(cur.State, profile, segDist, cfg)
			} else {
				deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100
				distMilli := int32(g.EdgeDist[ei]) * 10
				hr = propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			}

			// Slope-weighted costfactor interpolation
			distM := float32(g.EdgeDist[ei]) / 100.0
			edgeCost := computeEdgeCostWeighted(ecf, distM, hr, tc, cur.LastWayClass)
			newCost := cur.Cost + edgeCost
			effectiveCost := newCost + hr.State.elevationCorrection(cfg)

			// Absolute pruning (effective, not raw — maxTotalCost is effective)
			if effectiveCost > maxTotalCost {
				continue
			}

			// Pruning: bounded early-exit + dominance check
			if state.nodeGen[target] == state.generation {
				if newCost > state.bestCost[target]+cfg.MaxLatentCost {
					continue
				}
				if effectiveCost >= state.bestCost[target] {
					continue
				}
			}

			state.bestCost[target] = effectiveCost
			state.nodeGen[target] = state.generation
			state.parentNode[target] = cur.NodeID
			state.parentEdge[target] = ei

			// Dijkstra: AirDist = 0 (c=0)
			state.frontier.Push(FrontierEntry{
				NodeID:       target,
				Cost:         newCost,
				State:        hr.State,
				Parent:       cur.NodeID,
				Edge:         ei,
				LastEdgeDX:   newDX,
				LastEdgeDY:   newDY,
				LastWayClass: ecf.WayClass,
			})
		}
	}

	return nil, fmt.Errorf("no route found (pass1: expanded %d nodes, maxCost=%.0f)", expansions, maxTotalCost)
}

// extractTrack walks parent pointers backwards from dst to src.
// maxNodes guards against infinite loops from corrupt parent pointers;
// if exhausted, the track is truncated (should not happen with a valid route).
func extractTrack(state *RouterState, src, dst uint32, maxNodes uint32) []uint32 {
	var track []uint32
	node := dst
	limit := int(maxNodes)
	for node != src && limit > 0 {
		track = append(track, node)
		node = state.parentNode[node]
		limit--
	}
	track = append(track, src)
	// Reverse
	for i, j := 0, len(track)-1; i < j; i, j = i+1, j-1 {
		track[i], track[j] = track[j], track[i]
	}
	return track
}

// buildTrackIndex creates a map[nodeID]→remainingCost from the pass0 track.
// remainingCost is the cost from that node to dst along the pass0 track.
func buildTrackIndex(state *RouterState, track []uint32) map[uint32]float32 {
	if len(track) == 0 {
		return nil
	}

	index := make(map[uint32]float32, len(track))
	totalCost := state.bestCost[track[len(track)-1]] // effective cost at dst
	for _, n := range track {
		// remainingCost = totalCost - costAtNode
		// Clamp to 0: inadmissible A* (pass0) can overestimate node costs,
		// producing negative remaining values that would break cost-cutting.
		if state.nodeGen[n] == state.generation {
			remaining := totalCost - state.bestCost[n]
			if remaining < 0 {
				remaining = 0
			}
			index[n] = remaining
		}
	}
	return index
}

// retrack reconstructs the path from parent pointers and builds the RouteResult.
func retrack(g *graph.Graph, state *RouterState, src, dst uint32, cost float64) *RouteResult {
	result := buildRetrackResult(g, src, dst,
		state.parentNode, state.parentEdge,
		state.nodeGen, state.generation)
	result.Cost = cost
	return result
}

// --- Sparse variants ---
// NOTE: These intentionally duplicate the flat-array pass0/pass1/retrack logic.
// Flat arrays use generation counters + direct indexing; sparse maps use Go maps.
// The access patterns differ enough that a shared abstraction would hurt readability
// and performance. Phase 4 may extract a generic searchLoop if Go generics help.

// pass0Sparse is the sparse-map variant of pass0.
func pass0Sparse(ctx context.Context, g *graph.Graph, state *SparseState,
	src, dst uint32, cfg *CostConfig) ([]uint32, float32, *PassStats, error) {

	start := time.Now()
	state.frontier.Reset()

	dstLat := g.NodeLat[dst]
	dstLon := g.NodeLon[dst]

	airDist := float32(cfg.Pass0Coefficient) * equirectDist(g.NodeLat[src], g.NodeLon[src], dstLat, dstLon)
	state.frontier.Push(FrontierEntry{
		NodeID:  src,
		Cost:    0,
		AirDist: airDist,
		Parent:  src,
	})
	state.bestCost[src] = 0

	expansions := 0

	for state.frontier.Len() > 0 {
		if expansions&(checkCancelEvery-1) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, nil, err
			}
		}
		expansions++
		if expansions > maxExpansions {
			return nil, 0, nil, fmt.Errorf("pass0Sparse: expansion limit reached (%d)", maxExpansions)
		}

		cur := state.frontier.Pop()

		// Skip stale entries (must come before destination check)
		if best, ok := state.bestCost[cur.NodeID]; ok {
			if cur.Cost+cur.State.elevationCorrection(cfg) > best {
				continue
			}
		}

		// Reached destination
		if cur.NodeID == dst {
			track := extractTrackSparse(state, src, dst, g.NodeCount)
			stats := &PassStats{
				Pass0Expansions: expansions,
				Pass0DurationMs: time.Since(start).Milliseconds(),
			}
			// Return effective cost (not raw) — must match buildTrackIndexSparse semantics
			effectiveCost := cur.Cost + cur.State.elevationCorrection(cfg)
			return track, effectiveCost, stats, nil
		}

		edgeStart := g.Offsets[cur.NodeID]
		edgeEnd := g.Offsets[cur.NodeID+1]
		for ei := edgeStart; ei < edgeEnd; ei++ {
			target := g.Targets[ei]

			ecf := cfg.EdgeCost(g, ei)
			if ecf.Flat >= costBlocked {
				continue
			}

			// Turncost + direction
			tc, newDX, newDY := computeTurnCost(
				g.NodeLon[cur.NodeID], g.NodeLat[cur.NodeID],
				g.NodeLon[target], g.NodeLat[target],
				cur.LastEdgeDX, cur.LastEdgeDY, cfg.TurnCost, g.EdgeDist[ei])

			// Hysteresis
			var hr HysteresisResult
			profStart := g.EdgeElevOff[ei]
			profEnd := g.EdgeElevOff[ei+1]
			if profEnd > profStart && profEnd-profStart <= 5000 {
				profile := g.EdgeElevProf[profStart:profEnd]
				segDist := edgeSegmentDistances(g, ei)
				hr = propagateHysteresisProfile(cur.State, profile, segDist, cfg)
			} else {
				deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100
				distMilli := int32(g.EdgeDist[ei]) * 10
				hr = propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			}

			// Slope-weighted costfactor interpolation
			distM := float32(g.EdgeDist[ei]) / 100.0
			edgeCost := computeEdgeCostWeighted(ecf, distM, hr, tc, cur.LastWayClass)
			newCost := cur.Cost + edgeCost
			effectiveCost := newCost + hr.State.elevationCorrection(cfg)

			// Pruning: bounded early-exit + dominance check
			if best, ok := state.bestCost[target]; ok {
				if newCost > best+cfg.MaxLatentCost {
					continue
				}
				if effectiveCost >= best {
					continue
				}
			}

			state.bestCost[target] = effectiveCost
			state.parentNode[target] = cur.NodeID
			state.parentEdge[target] = ei

			airD := float32(cfg.Pass0Coefficient) * equirectDist(g.NodeLat[target], g.NodeLon[target], dstLat, dstLon)
			state.frontier.Push(FrontierEntry{
				NodeID:       target,
				Cost:         newCost,
				AirDist:      airD,
				State:        hr.State,
				Parent:       cur.NodeID,
				Edge:         ei,
				LastEdgeDX:   newDX,
				LastEdgeDY:   newDY,
				LastWayClass: ecf.WayClass,
			})
		}
	}

	return nil, 0, nil, fmt.Errorf("no route found (pass0Sparse: expanded %d nodes)", expansions)
}

// pass1Sparse is the sparse-map variant of pass1.
// trackIndex must be built BEFORE clearMaps (pass0 data is in the maps).
func pass1Sparse(ctx context.Context, g *graph.Graph, state *SparseState,
	src, dst uint32, cfg *CostConfig,
	trackIndex map[uint32]float32, maxTotalCost float32) (*PassStats, error) {

	start := time.Now()
	state.frontier.Reset()

	state.frontier.Push(FrontierEntry{
		NodeID: src,
		Cost:   0,
		Parent: src,
	})
	state.bestCost[src] = 0

	expansions := 0

	for state.frontier.Len() > 0 {
		if expansions&(checkCancelEvery-1) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		expansions++
		if expansions > maxExpansions {
			return nil, fmt.Errorf("pass1Sparse: expansion limit reached (%d)", maxExpansions)
		}

		cur := state.frontier.Pop()

		effectiveCur := cur.Cost + cur.State.elevationCorrection(cfg)

		// Skip stale entries BEFORE destination check and cost-cutting — ensures
		// only settled (non-superseded) entries tighten maxTotalCost, and prevents
		// returning a suboptimal destination entry superseded by a better path.
		if best, ok := state.bestCost[cur.NodeID]; ok {
			if effectiveCur > best {
				continue
			}
		}

		// Reached destination (after stale check — guarantees optimal)
		if cur.NodeID == dst {
			return &PassStats{
				Pass1Expansions: expansions,
				Pass1DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// Cost-cutting: if pass1 reaches a node on the pass0 track with lower cost,
		// the predicted total (effectiveCur + remaining along pass0 track) tightens the bound.
		// SAFETY MARGIN: pass0 is inadmissible (c>1, typically 3.0 for trekking) so its track
		// costs are heavily distorted — remaining from pass0 can underestimate the actual
		// remaining cost by 15-20%. Use pass0Coefficient as margin to compensate.
		if remaining, ok := trackIndex[cur.NodeID]; ok {
			cutoff := effectiveCur + remaining*cfg.Pass0Coefficient
			if cutoff < maxTotalCost {
				maxTotalCost = cutoff
			}
		}

		if effectiveCur > maxTotalCost {
			continue
		}

		edgeStart := g.Offsets[cur.NodeID]
		edgeEnd := g.Offsets[cur.NodeID+1]
		for ei := edgeStart; ei < edgeEnd; ei++ {
			target := g.Targets[ei]

			ecf := cfg.EdgeCost(g, ei)
			if ecf.Flat >= costBlocked {
				continue
			}

			// Turncost + direction
			tc, newDX, newDY := computeTurnCost(
				g.NodeLon[cur.NodeID], g.NodeLat[cur.NodeID],
				g.NodeLon[target], g.NodeLat[target],
				cur.LastEdgeDX, cur.LastEdgeDY, cfg.TurnCost, g.EdgeDist[ei])

			// Hysteresis
			var hr HysteresisResult
			profStart := g.EdgeElevOff[ei]
			profEnd := g.EdgeElevOff[ei+1]
			if profEnd > profStart && profEnd-profStart <= 5000 {
				profile := g.EdgeElevProf[profStart:profEnd]
				segDist := edgeSegmentDistances(g, ei)
				hr = propagateHysteresisProfile(cur.State, profile, segDist, cfg)
			} else {
				deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100
				distMilli := int32(g.EdgeDist[ei]) * 10
				hr = propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			}

			// Slope-weighted costfactor interpolation
			distM := float32(g.EdgeDist[ei]) / 100.0
			edgeCost := computeEdgeCostWeighted(ecf, distM, hr, tc, cur.LastWayClass)
			newCost := cur.Cost + edgeCost
			effectiveCost := newCost + hr.State.elevationCorrection(cfg)

			// Absolute pruning (effective, not raw — maxTotalCost is effective)
			if effectiveCost > maxTotalCost {
				continue
			}

			// Pruning: bounded early-exit + dominance check
			if best, ok := state.bestCost[target]; ok {
				if newCost > best+cfg.MaxLatentCost {
					continue
				}
				if effectiveCost >= best {
					continue
				}
			}

			state.bestCost[target] = effectiveCost
			state.parentNode[target] = cur.NodeID
			state.parentEdge[target] = ei

			state.frontier.Push(FrontierEntry{
				NodeID:       target,
				Cost:         newCost,
				State:        hr.State,
				Parent:       cur.NodeID,
				Edge:         ei,
				LastEdgeDX:   newDX,
				LastEdgeDY:   newDY,
				LastWayClass: ecf.WayClass,
			})
		}
	}

	return nil, fmt.Errorf("no route found (pass1Sparse: expanded %d nodes)", expansions)
}

// extractTrackSparse walks parent pointers backwards from dst to src (sparse variant).
// NOTE: Go maps return 0 for missing keys. Since node 0 is a valid graph node,
// a corrupt parent chain could silently route through node 0. The limit guard
// prevents infinite loops; correct routing guarantees a valid parent chain.
func extractTrackSparse(state *SparseState, src, dst uint32, maxNodes uint32) []uint32 {
	var track []uint32
	node := dst
	limit := int(maxNodes)
	for node != src && limit > 0 {
		track = append(track, node)
		node = state.parentNode[node]
		limit--
	}
	track = append(track, src)
	for i, j := 0, len(track)-1; i < j; i, j = i+1, j-1 {
		track[i], track[j] = track[j], track[i]
	}
	return track
}

func buildTrackIndexSparse(state *SparseState, track []uint32) map[uint32]float32 {
	if len(track) == 0 {
		return nil
	}
	index := make(map[uint32]float32, len(track))
	totalCost := state.bestCost[track[len(track)-1]]
	for _, n := range track {
		if cost, ok := state.bestCost[n]; ok {
			remaining := totalCost - cost
			if remaining < 0 {
				remaining = 0
			}
			index[n] = remaining
		}
	}
	return index
}

// retrackSparse reconstructs from sparse parent maps.
func retrackSparse(g *graph.Graph, state *SparseState, src, dst uint32, cost float64) *RouteResult {
	// Walk backwards with safety limit
	var path []uint32
	var edges []uint32
	node := dst
	maxIter := int(g.NodeCount)
	for node != src && maxIter > 0 {
		path = append(path, node)
		edges = append(edges, state.parentEdge[node])
		node = state.parentNode[node]
		maxIter--
	}
	path = append(path, src)

	// Reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	for i, j := 0, len(edges)-1; i < j; i, j = i+1, j-1 {
		edges[i], edges[j] = edges[j], edges[i]
	}

	// Build coordinates and compute metrics — expand edge geometry
	var coords []LatLonEle
	var totalDist, ascent, descent float64
	// edgeCoordStart[i] = coordinate index where edge i starts (the network node before it)
	edgeCoordStart := make([]int, len(edges))

	for i, n := range path {
		// Add the network node itself
		coords = append(coords, LatLonEle{
			Lat: float64(g.NodeLat[n]) / 1e7,
			Lon: float64(g.NodeLon[n]) / 1e7,
			Ele: float64(g.NodeEle[n]) / 10.0,
		})

		// If not the last node, record edge start and expand edge geometry
		if i < len(edges) {
			edgeCoordStart[i] = len(coords) - 1

			ei := edges[i]
			geomStart := g.EdgeGeomOff[ei]
			geomEnd := g.EdgeGeomOff[ei+1]
			if g.EdgeGeomLat != nil && geomEnd > geomStart+2 { // at least one interior point
				for gi := geomStart + 1; gi < geomEnd-1; gi++ {
					lat := float64(g.EdgeGeomLat[gi]) / 1e7
					lon := float64(g.EdgeGeomLon[gi]) / 1e7
					var ele float64
					profStart := g.EdgeElevOff[ei]
					profIdx := gi - geomStart
					if profStart+profIdx < g.EdgeElevOff[ei+1] {
						ele = float64(g.EdgeElevProf[profStart+profIdx]) / 10.0
					}
					coords = append(coords, LatLonEle{Lat: lat, Lon: lon, Ele: ele})
				}
			}
		}
	}

	// Compute distance and elevation from expanded coords
	for i := 1; i < len(coords); i++ {
		totalDist += haversineDist(coords[i-1].Lat, coords[i-1].Lon,
			coords[i].Lat, coords[i].Lon)
		dEle := coords[i].Ele - coords[i-1].Ele
		if dEle > 0 {
			ascent += dEle
		} else {
			descent -= dEle
		}
	}

	// Build segments using expanded coordinate indices
	var segments []RouteSegment
	if len(edges) > 0 && g.EdgeOSMIdx != nil {
		curOSMIdx := g.EdgeOSMIdx[edges[0]]
		curTags := g.EdgeTags1[edges[0]]
		segFromCoord := edgeCoordStart[0]

		for i := 1; i < len(edges); i++ {
			osmIdx := g.EdgeOSMIdx[edges[i]]
			if osmIdx != curOSMIdx {
				seg := makeSegment(g, segFromCoord, edgeCoordStart[i], curTags, curOSMIdx)
				segments = append(segments, seg)
				segFromCoord = edgeCoordStart[i]
				curOSMIdx = osmIdx
				curTags = g.EdgeTags1[edges[i]]
			}
		}
		seg := makeSegment(g, segFromCoord, len(coords)-1, curTags, curOSMIdx)
		segments = append(segments, seg)
	}

	return &RouteResult{
		Coordinates: coords,
		DistanceM:   totalDist,
		AscentM:     ascent,
		DescentM:    descent,
		Cost:        cost,
		Segments:    segments,
	}
}
