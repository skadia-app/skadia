// pass.go
package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

const checkCancelEvery = 4096

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

			edgeCost := cfg.EdgeCost(g, ei)

			// Unit conversion: dm → mm
			deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100 // dm → mm
			distMilli := int32(g.EdgeDist[ei]) * 100                           // dm → mm

			newState, elevCost := propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			newCost := cur.Cost + edgeCost + elevCost
			effectiveCost := newCost + newState.elevationCorrection(cfg)

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
				NodeID:  target,
				Cost:    newCost,
				AirDist: airD,
				State:   newState,
				Parent:  cur.NodeID,
				Edge:    ei,
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

		cur := state.frontier.Pop()

		// Reached destination
		if cur.NodeID == dst {
			return &PassStats{
				Pass1Expansions: expansions,
				Pass1DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// Compute effective cost (raw + latent hysteresis buffer).
		// Note: cur.Cost is RAW, bestCost stores EFFECTIVE.
		effectiveCur := cur.Cost + cur.State.elevationCorrection(cfg)

		// Skip stale entries BEFORE cost-cutting — ensures only settled
		// (non-superseded) entries tighten maxTotalCost for optimal bounds.
		if state.nodeGen[cur.NodeID] == state.generation &&
			effectiveCur > state.bestCost[cur.NodeID] {
			continue
		}

		// Cost-cutting: if pass1 reaches a node on the pass0 track with lower cost,
		// the predicted total (effectiveCur + remaining along pass0 track) tightens the bound.
		if remaining, ok := trackIndex[cur.NodeID]; ok {
			cutoff := effectiveCur + remaining
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

			edgeCost := cfg.EdgeCost(g, ei)
			deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100 // dm → mm
			distMilli := int32(g.EdgeDist[ei]) * 100                           // dm → mm

			newState, elevCost := propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			newCost := cur.Cost + edgeCost + elevCost
			effectiveCost := newCost + newState.elevationCorrection(cfg)

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
				NodeID: target,
				Cost:   newCost,
				State:  newState,
				Parent: cur.NodeID,
				Edge:   ei,
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
		if state.nodeGen[n] == state.generation {
			index[n] = totalCost - state.bestCost[n]
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

			edgeCost := cfg.EdgeCost(g, ei)
			deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100 // dm → mm
			distMilli := int32(g.EdgeDist[ei]) * 100                           // dm → mm

			newState, elevCost := propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			newCost := cur.Cost + edgeCost + elevCost
			effectiveCost := newCost + newState.elevationCorrection(cfg)

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
				NodeID:  target,
				Cost:    newCost,
				AirDist: airD,
				State:   newState,
				Parent:  cur.NodeID,
				Edge:    ei,
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

		cur := state.frontier.Pop()

		if cur.NodeID == dst {
			return &PassStats{
				Pass1Expansions: expansions,
				Pass1DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}

		effectiveCur := cur.Cost + cur.State.elevationCorrection(cfg)

		// Skip stale entries BEFORE cost-cutting — ensures only settled
		// (non-superseded) entries tighten maxTotalCost for optimal bounds.
		if best, ok := state.bestCost[cur.NodeID]; ok {
			if effectiveCur > best {
				continue
			}
		}

		// Cost-cutting: if pass1 reaches a node on the pass0 track with lower cost,
		// the predicted total (effectiveCur + remaining along pass0 track) tightens the bound.
		if remaining, ok := trackIndex[cur.NodeID]; ok {
			cutoff := effectiveCur + remaining
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

			edgeCost := cfg.EdgeCost(g, ei)
			deltaEleMilli := (g.NodeEle[target] - g.NodeEle[cur.NodeID]) * 100 // dm → mm
			distMilli := int32(g.EdgeDist[ei]) * 100                           // dm → mm

			newState, elevCost := propagateHysteresis(cur.State, deltaEleMilli, distMilli, cfg)
			newCost := cur.Cost + edgeCost + elevCost
			effectiveCost := newCost + newState.elevationCorrection(cfg)

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
				NodeID: target,
				Cost:   newCost,
				State:  newState,
				Parent: cur.NodeID,
				Edge:   ei,
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
			index[n] = totalCost - cost
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

	// Build coordinates
	coords := make([]LatLonEle, len(path))
	var totalDist, ascent, descent float64
	for i, n := range path {
		coords[i] = LatLonEle{
			Lat: float64(g.NodeLat[n]) / 1e7,
			Lon: float64(g.NodeLon[n]) / 1e7,
			Ele: float64(g.NodeEle[n]) / 10.0,
		}
		if i > 0 {
			totalDist += haversineDist(coords[i-1].Lat, coords[i-1].Lon,
				coords[i].Lat, coords[i].Lon)
			dEle := coords[i].Ele - coords[i-1].Ele
			if dEle > 0 {
				ascent += dEle
			} else {
				descent -= dEle
			}
		}
	}

	var segments []RouteSegment
	if len(edges) > 0 {
		curOSMIdx := g.EdgeOSMIdx[edges[0]]
		curTags := g.EdgeTags1[edges[0]]
		segStart := 0
		for i := 1; i < len(edges); i++ {
			osmIdx := g.EdgeOSMIdx[edges[i]]
			if osmIdx != curOSMIdx {
				segments = append(segments, makeSegment(g, segStart, i, curTags, curOSMIdx))
				segStart = i
				curOSMIdx = osmIdx
				curTags = g.EdgeTags1[edges[i]]
			}
		}
		segments = append(segments, makeSegment(g, segStart, len(edges), curTags, curOSMIdx))
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
