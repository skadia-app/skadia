// tilerouter.go — TileRouter: single-tile and cross-tile A* routing.
package routing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// TileRouter performs A* routing across a tiled graph.
// Single-tile routes delegate to the existing Router.
// Cross-tile routes use a sparse A* with BorderNode expansion.
type TileRouter struct {
	mgr                  *TileManager // on-demand tile loading (nil in tests)
	cache                *TileCache   // direct cache access (used in tests without TileManager)
	maxExpansionsPerTile uint32
	sparseSem            chan struct{}
}

// NewTileRouter creates a TileRouter backed by a TileManager for on-demand loading.
func NewTileRouter(mgr *TileManager) *TileRouter {
	return &TileRouter{
		mgr:                  mgr,
		cache:                mgr.cache,
		maxExpansionsPerTile: 500_000,
		sparseSem:            make(chan struct{}, 10),
	}
}

// newTileRouterFromCache creates a TileRouter backed only by a cache (no disk loading).
// Used in tests where tiles are pre-populated in memory.
func newTileRouterFromCache(cache *TileCache) *TileRouter {
	return &TileRouter{
		cache:                cache,
		maxExpansionsPerTile: 500_000,
		sparseSem:            make(chan struct{}, 10),
	}
}

// Route computes a route from req.From to req.To, loading tiles on demand.
func (tr *TileRouter) Route(ctx context.Context, req RouteRequest,
	cfg *CostConfig) (*RouteResult, error) {

	startTileID := graph.TileIDFromCoord(req.FromLat, req.FromLon)
	endTileID := graph.TileIDFromCoord(req.ToLat, req.ToLon)

	if startTileID == endTileID {
		return tr.routeSingleTile(ctx, startTileID, req, cfg)
	}
	return tr.routeCrossTile(ctx, startTileID, endTileID, req, cfg)
}

// getTile loads a tile via TileManager (on-demand from disk) or falls back to
// direct cache lookup (for tests without TileManager).
func (tr *TileRouter) getTile(tileID uint32) (*graph.Graph, error) {
	if tr.mgr != nil {
		return tr.mgr.GetTile(tileID)
	}
	// Test-only path: direct cache access.
	g, ok := tr.cache.Get(tileID)
	if !ok {
		return nil, fmt.Errorf("tile %d not in cache", tileID)
	}
	return g, nil
}

// routeSingleTile handles routing within a single tile by delegating to
// the existing sparse Router implementation.
func (tr *TileRouter) routeSingleTile(ctx context.Context, tileID uint32,
	req RouteRequest, cfg *CostConfig) (*RouteResult, error) {

	g, err := tr.getTile(tileID)
	if err != nil {
		return nil, fmt.Errorf("single-tile route: %w", err)
	}

	// Concurrency control via sparseSem.
	select {
	case tr.sparseSem <- struct{}{}:
		defer func() { <-tr.sparseSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	r := NewRouter(g, 0, 1)
	return r.Route(ctx, req, cfg)
}

// routeCrossTile runs a single-pass sparse A* across tile boundaries.
// Border nodes are matched between tiles via CoordKey → BorderNodeIndex lookup.
func (tr *TileRouter) routeCrossTile(ctx context.Context,
	startTileID, endTileID uint32,
	req RouteRequest, cfg *CostConfig) (*RouteResult, error) {

	totalStart := time.Now()

	// Load start and end tiles.
	gStart, err := tr.getTile(startTileID)
	if err != nil {
		return nil, fmt.Errorf("cross-tile: start tile: %w", err)
	}
	gEnd, err := tr.getTile(endTileID)
	if err != nil {
		return nil, fmt.Errorf("cross-tile: end tile: %w", err)
	}

	// Snap to nearest nodes.
	srcLocal, srcDist := graph.NearestNode(gStart, req.FromLat, req.FromLon)
	dstLocal, dstDist := graph.NearestNode(gEnd, req.ToLat, req.ToLon)

	const maxSnapDist uint16 = 50000 // 5km in decimeters
	if srcDist >= maxSnapDist {
		return nil, fmt.Errorf("no graph node within 5km of start coordinate")
	}
	if dstDist >= maxSnapDist {
		return nil, fmt.Errorf("no graph node within 5km of destination coordinate")
	}

	srcGID := MakeGlobalNodeID(startTileID, srcLocal)
	dstGID := MakeGlobalNodeID(endTileID, dstLocal)

	if srcGID == dstGID {
		lat := float64(gStart.NodeLat[srcLocal]) / 1e7
		lon := float64(gStart.NodeLon[srcLocal]) / 1e7
		ele := float64(gStart.NodeEle[srcLocal]) / 10.0
		return &RouteResult{
			Coordinates: []LatLonEle{{Lat: lat, Lon: lon, Ele: ele}},
		}, nil
	}

	// Destination coordinates for A* heuristic.
	dstLat := float64(gEnd.NodeLat[dstLocal]) / 1e7
	dstLon := float64(gEnd.NodeLon[dstLocal]) / 1e7

	// A* state.
	state := newCrossTileState()

	// Seed source node.
	srcNS := state.getOrCreate(srcGID)
	srcNS.BestCost = 0
	srcNS.ParentNode = srcGID // self-parent sentinel

	srcLat := float64(gStart.NodeLat[srcLocal]) / 1e7
	srcLon := float64(gStart.NodeLon[srcLocal]) / 1e7
	airDist := float32(equirectDistM(srcLat, srcLon, dstLat, dstLon)) * cfg.Pass0Coefficient

	state.frontier.Push(CrossFrontierEntry{
		GlobalNodeID: srcGID,
		Cost:         0,
		AirDist:      airDist,
		State:        PathState{},
		TileID:       startTileID,
	})

	// Track hysteresis state per global node.
	pathStates := make(map[uint64]PathState)
	pathStates[srcGID] = PathState{}

	var expansions int
	maxExpansions := int(tr.maxExpansionsPerTile) * 20 // generous cross-tile budget

	for state.frontier.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		entry := state.frontier.Pop()
		gid := entry.GlobalNodeID

		ns := state.getOrCreate(gid)
		if entry.Cost > ns.BestCost {
			continue // stale entry
		}

		if gid == dstGID {
			// Found destination — retrace.
			result := tr.retrackCrossTile(state, srcGID, dstGID)
			result.Cost = float64(ns.BestCost)
			result.PassStats = PassStats{
				Pass0Expansions: expansions,
				TotalDurationMs: time.Since(totalStart).Milliseconds(),
			}
			return result, nil
		}

		expansions++
		if expansions > maxExpansions {
			return nil, fmt.Errorf("cross-tile A*: expansion limit reached (%d)", maxExpansions)
		}

		curTileID := entry.TileID
		_, localNode := SplitGlobalNodeID(gid)

		g, err := tr.getTile(curTileID)
		if err != nil {
			slog.Warn("cross-tile: cannot load tile for expansion",
				"tile_id", curTileID, "err", err)
			continue
		}

		hyState := entry.State

		// Expand edges.
		edgeStart := g.Offsets[localNode]
		edgeEnd := g.Offsets[localNode+1]
		for ei := edgeStart; ei < edgeEnd; ei++ {
			target := g.Targets[ei]

			edgeCost := cfg.EdgeCost(g, ei)
			// Skip blocked edges. EdgeCost returns distM * costBlocked (=10_000)
			// for impassable highways, so a 100m blocked edge costs 1M.
			// Threshold costBlocked*100 = 1M catches all blocked edges ≥100m.
			if edgeCost >= costBlocked*100 {
				continue
			}

			// Elevation hysteresis.
			deltaEleMilli := (g.NodeEle[target] - g.NodeEle[localNode]) * 100 // dm → mm
			distMilli := int32(g.EdgeDist[ei]) * 10                           // cm → mm
			newHyState, elevCost := propagateHysteresis(hyState, deltaEleMilli, distMilli, cfg)

			totalEdgeCost := edgeCost + elevCost
			newCost := entry.Cost + totalEdgeCost
			correctedCost := newCost + newHyState.elevationCorrection(cfg)

			targetLat := float64(g.NodeLat[target]) / 1e7
			targetLon := float64(g.NodeLon[target]) / 1e7

			// Always add to current tile.
			targetGID := MakeGlobalNodeID(curTileID, target)
			tns := state.getOrCreate(targetGID)

			if correctedCost < tns.BestCost {
				tns.BestCost = correctedCost
				tns.ParentNode = gid
				tns.ParentEdge = ei
				tns.ParentEdgeTile = curTileID

				targetAirDist := float32(equirectDistM(targetLat, targetLon, dstLat, dstLon)) * cfg.Pass0Coefficient
				pathStates[targetGID] = newHyState

				state.frontier.Push(CrossFrontierEntry{
					GlobalNodeID: targetGID,
					Cost:         correctedCost,
					AirDist:      targetAirDist,
					State:        newHyState,
					TileID:       curTileID,
				})
			}

			// If this target is a border node, also push into neighboring tiles
			// that contain the same coordinate.
			if g.IsBorder != nil && len(g.IsBorder) > int(target) && g.IsBorder[target] {
				for _, neighborTileID := range graph.NeighborTileIDs(curTileID) {
					tr.expandBorderNode(state, pathStates, g, target, curTileID,
						neighborTileID, gid, ei, correctedCost, newHyState, dstLat, dstLon, cfg)
				}
			}
		}
	}

	return nil, fmt.Errorf("cross-tile A*: no route found (expanded %d nodes)", expansions)
}

// expandBorderNode looks up the corresponding node in the neighbor tile and
// pushes it onto the frontier if it improves cost.
func (tr *TileRouter) expandBorderNode(
	state *CrossTileState,
	pathStates map[uint64]PathState,
	srcGraph *graph.Graph,
	localTarget uint32,
	srcTileID, neighborTileID uint32,
	parentGID uint64,
	edgeIdx uint32,
	newCost float32,
	hyState PathState,
	dstLat, dstLon float64,
	cfg *CostConfig,
) {
	neighborGraph, err := tr.getTile(neighborTileID)
	if err != nil {
		return // neighbor tile not available
	}

	if neighborGraph.BorderIndex == nil {
		return
	}

	// Build coordinate key from the target node in the source graph.
	coordKey := graph.CoordKeyToInt64(graph.CoordKey{
		Lat: srcGraph.NodeLat[localTarget],
		Lon: srcGraph.NodeLon[localTarget],
	})

	neighborLocalID, found := neighborGraph.BorderIndex.Lookup(coordKey)
	if !found {
		return
	}

	neighborGID := MakeGlobalNodeID(neighborTileID, neighborLocalID)
	tns := state.getOrCreate(neighborGID)

	// newCost already includes elevationCorrection from the caller — do NOT apply again.
	if newCost < tns.BestCost {
		tns.BestCost = newCost
		tns.ParentNode = parentGID
		tns.ParentEdge = edgeIdx
		tns.ParentEdgeTile = srcTileID // edge belongs to source tile, not neighbor

		nLat := float64(neighborGraph.NodeLat[neighborLocalID]) / 1e7
		nLon := float64(neighborGraph.NodeLon[neighborLocalID]) / 1e7
		airDist := float32(equirectDistM(nLat, nLon, dstLat, dstLon)) * cfg.Pass0Coefficient

		pathStates[neighborGID] = hyState

		state.frontier.Push(CrossFrontierEntry{
			GlobalNodeID: neighborGID,
			Cost:         newCost,
			AirDist:      airDist,
			State:        hyState,
			TileID:       neighborTileID,
		})
	}
}

// retrackCrossTile walks the parent chain from dst back to src, collecting
// network-node coordinates. This is a STUB: it does not expand edge geometry.
func (tr *TileRouter) retrackCrossTile(state *CrossTileState,
	srcGID, dstGID uint64) *RouteResult {

	// Walk parent chain.
	var chain []uint64
	cur := dstGID
	maxIter := len(state.nodes) + 1
	for cur != srcGID && maxIter > 0 {
		chain = append(chain, cur)
		ns, ok := state.nodes[cur]
		if !ok {
			break
		}
		cur = ns.ParentNode
		maxIter--
	}
	chain = append(chain, srcGID)

	// Reverse to src→dst order.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}

	// Build coordinates and compute metrics.
	coords := make([]LatLonEle, 0, len(chain))
	var totalDist, ascent, descent float64

	for _, gid := range chain {
		tileID, localNode := SplitGlobalNodeID(gid)
		g, err := tr.getTile(tileID)
		if err != nil {
			continue
		}
		if localNode >= g.NodeCount {
			continue
		}
		c := LatLonEle{
			Lat: float64(g.NodeLat[localNode]) / 1e7,
			Lon: float64(g.NodeLon[localNode]) / 1e7,
			Ele: float64(g.NodeEle[localNode]) / 10.0,
		}
		if len(coords) > 0 {
			prev := coords[len(coords)-1]
			totalDist += haversineDist(prev.Lat, prev.Lon, c.Lat, c.Lon)
			dEle := c.Ele - prev.Ele
			if dEle > 0 {
				ascent += dEle
			} else {
				descent -= dEle
			}
		}
		coords = append(coords, c)
	}

	return &RouteResult{
		Coordinates: coords,
		DistanceM:   totalDist,
		AscentM:     ascent,
		DescentM:    descent,
	}
}

