package routing

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// landmarkDijkstraUnified runs a flat-CSR Dijkstra from src over the unified graph.
// No hysteresis (Set D: UphillCost=0). Returns costs for all dense nodes.
func landmarkDijkstraUnified(ctx context.Context, csr *graph.UnifiedCSR, src uint32) []float32 {
	costs := make([]float32, csr.NodeCount)
	for i := range costs {
		costs[i] = math.MaxFloat32
	}
	costs[src] = 0

	frontier := &Frontier{}
	frontier.Push(FrontierEntry{NodeID: uint64(src), Cost: 0})

	expansions := 0
	for frontier.Len() > 0 {
		entry := frontier.Pop()
		nd := uint32(entry.NodeID) // Safe: denseID < uint32 max
		if entry.Cost > costs[nd]+0.001 {
			continue
		}

		expansions++
		if expansions%5_000_000 == 0 {
			if ctx.Err() != nil {
				break
			}
			slog.Info("landmark dijkstra progress", "expansions", expansions, "frontier", frontier.Len())
		}

		for ei := csr.Offsets[nd]; ei < csr.Offsets[nd+1]; ei++ {
			edge := csr.Edges[ei]
			if edge.CostFactor >= costBlocked {
				continue
			}
			newCost := costs[nd] + float32(edge.Dist)/100.0*edge.CostFactor
			if newCost < costs[edge.Target] {
				costs[edge.Target] = newCost
				frontier.Push(FrontierEntry{NodeID: uint64(edge.Target), Cost: newCost})
			}
		}
	}

	slog.Info("landmark dijkstra complete", "expansions", expansions)
	return costs
}

// BuildLandmarkEdgeCostClosureCSR wraps BuildLandmarkEdgeCostClosure to return
// a graph.EdgeCostFunc (graph.EdgeCostFactors) instead of routing.EdgeCostFactors.
// The WayClass field from routing.EdgeCostFactors is dropped since the CSR
// PrecomputedEdge only stores the min costfactor.
func BuildLandmarkEdgeCostClosureCSR() graph.EdgeCostFunc {
	inner := BuildLandmarkEdgeCostClosure()
	return func(tags1, tags2 uint64) graph.EdgeCostFactors {
		ecf := inner(tags1, tags2)
		return graph.EdgeCostFactors{
			Flat: ecf.Flat,
			Up:   ecf.Up,
			Down: ecf.Down,
		}
	}
}

// RunUnifiedLandmarkPreprocessing builds a unified CSR from all tiles, selects
// 16 landmarks, runs Dijkstras, and writes distances back to tile files.
// This replaces the old per-tile DenseIndex approach with a single flat graph.
func RunUnifiedLandmarkPreprocessing(
	ctx context.Context,
	tileDir string,
	progressFn func(step string, current, total int),
) (*graph.LandmarkMetaData, error) {
	if progressFn == nil {
		progressFn = func(string, int, int) {}
	}
	t0 := time.Now()

	// 1. Build unified CSR from all tiles
	progressFn("building unified CSR", 0, 0)
	csr, nodeMapping, err := graph.BuildUnifiedCSR(ctx, tileDir, BuildLandmarkEdgeCostClosureCSR(), progressFn)
	if err != nil {
		return nil, fmt.Errorf("build unified CSR: %w", err)
	}
	defer csr.Close()

	slog.Info("unified landmark preprocessing: CSR built",
		"nodes", csr.NodeCount,
		"edges", csr.EdgeCount,
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	// 2. Scan tiles for lat/lon/ele (needed for SelectLandmarks + write-back)
	progressFn("scanning tiles", 0, 0)
	tiles, err := graph.ScanTilesForLandmarks(tileDir)
	if err != nil {
		return nil, fmt.Errorf("scan tiles: %w", err)
	}

	// 3. Select landmarks with farthestDijkstra closure
	progressFn("selecting landmarks", 0, 16)
	farthestDijkstra := func(src uint64) (graph.LandmarkCandidate, error) {
		srcTileID, srcLocalID := graph.SplitGlobalNodeID(src)
		mapping := nodeMapping[srcTileID]
		if mapping == nil || uint32(len(mapping)) <= srcLocalID {
			return graph.LandmarkCandidate{}, fmt.Errorf("node %d not in CSR mapping (tile %d, local %d)", src, srcTileID, srcLocalID)
		}
		srcDense := mapping[srcLocalID]

		costs := landmarkDijkstraUnified(ctx, csr, srcDense)

		// Find farthest reachable node
		var best graph.LandmarkCandidate
		var bestCost float32
		for _, tile := range tiles {
			tileMapping := nodeMapping[tile.TileID]
			if tileMapping == nil {
				continue
			}
			for localID := range tile.NodeCount {
				if localID >= uint32(len(tileMapping)) {
					continue
				}
				denseID := tileMapping[localID]
				cost := costs[denseID]
				if cost > bestCost && cost < math.MaxFloat32 {
					bestCost = cost
					best = graph.LandmarkCandidate{
						GlobalNodeID: graph.MakeGlobalNodeID(tile.TileID, localID),
						Lat:          tile.NodeLats[localID],
						Lon:          tile.NodeLons[localID],
						Ele:          tile.NodeEles[localID],
					}
				}
			}
		}
		return best, nil
	}

	meta, err := graph.SelectLandmarks(tiles, farthestDijkstra)
	if err != nil {
		return nil, fmt.Errorf("select landmarks: %w", err)
	}

	slog.Info("unified landmark preprocessing: landmarks selected",
		"count", meta.LandmarkCount,
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	// 4. Run 16 Dijkstras with errgroup.SetLimit(2)
	allCosts := make([][]float32, meta.LandmarkCount)
	var dijkstrasDone atomic.Int64
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(2)

	for lk := range meta.LandmarkCount {
		eg.Go(func() error {
			tileID, localID := graph.SplitGlobalNodeID(meta.NodeIDs[lk])
			mapping := nodeMapping[tileID]
			if mapping == nil || uint32(len(mapping)) <= localID {
				return fmt.Errorf("landmark %d node not in CSR mapping (tile %d, local %d)", lk, tileID, localID)
			}
			srcDense := mapping[localID]
			allCosts[lk] = landmarkDijkstraUnified(egCtx, csr, srcDense)
			done := dijkstrasDone.Add(1)
			slog.Info("landmark dijkstra complete", "landmark", lk, "run", done, "total", meta.LandmarkCount)
			progressFn("dijkstra runs", int(done), meta.LandmarkCount)
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, fmt.Errorf("landmark dijkstras: %w", err)
	}

	slog.Info("unified landmark preprocessing: all dijkstras complete",
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	// 5. Write back to tiles
	progressFn("writing landmark distances", 0, len(tiles))
	tileDistArrays := make(map[uint32][]float32, len(tiles))
	for _, tile := range tiles {
		dists := make([]float32, int(tile.NodeCount)*graph.LandmarksPerSet)
		for i := range dists {
			dists[i] = math.MaxFloat32
		}
		mapping := nodeMapping[tile.TileID]
		if mapping == nil {
			continue
		}
		for localID := range tile.NodeCount {
			if localID >= uint32(len(mapping)) {
				continue
			}
			denseID := mapping[localID]
			for lk := range meta.LandmarkCount {
				dists[int(localID)*graph.LandmarksPerSet+lk] = allCosts[lk][denseID]
			}
		}
		tileDistArrays[tile.TileID] = dists
	}

	if err := graph.WriteLandmarkSectionsForSet(tileDir, tiles, meta, graph.SectionLandmarkDistD, tileDistArrays); err != nil {
		return nil, fmt.Errorf("write landmark sections: %w", err)
	}

	slog.Info("unified landmark preprocessing complete",
		"landmarks", meta.LandmarkCount,
		"tiles", len(tiles),
		"totalElapsed", time.Since(t0).Round(time.Millisecond),
	)

	return meta, nil
}
