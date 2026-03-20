package routing

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// LandmarkCostConfig defines the elevation parameters for one landmark set.
type LandmarkCostConfig struct {
	SetID            LandmarkSetID
	UphillCost       float32
	DownhillCost     float32
	UphillCutoffPct  float32 // slope percentage (e.g. 3.0 = 3%)
	DownhillCutoffPct float32
	ElevPenaltyBuffer float32 // meters
	ElevMaxBuffer     float32 // meters
	ElevBufferReduce  float32 // mm drain per mm distance
}

// LandmarkCostConfigs defines the 4 elevation configurations for ALT landmark sets.
// All 4 share the same edge-cost closure (hiking base, enrichment=0) but differ
// in elevation hysteresis parameters. ElevBufferReduce = 0.01 ensures admissibility.
var LandmarkCostConfigs = [NumLandmarkSets]LandmarkCostConfig{
	{LandmarkSetA, 7, 7, 3.0, 3.0, 5, 10, 0.01},
	{LandmarkSetB, 7, 0, 3.0, 3.0, 5, 10, 0.01},
	{LandmarkSetC, 0, 7, 3.0, 3.0, 5, 10, 0.01},
	{LandmarkSetD, 0, 0, 3.0, 3.0, 5, 10, 0.01},
}

// BuildLandmarkCostConfig creates a CostConfig from a LandmarkCostConfig.
// The EdgeCost closure is shared across all sets (pure infrastructure cost).
func BuildLandmarkCostConfig(lcc LandmarkCostConfig, edgeCostFn func(tags1, tags2 uint64) EdgeCostFactors) *CostConfig {
	cfg := &CostConfig{
		UphillCost:        lcc.UphillCost,
		DownhillCost:      lcc.DownhillCost,
		UphillCutoff:      lcc.UphillCutoffPct / 100.0,
		DownhillCutoff:    lcc.DownhillCutoffPct / 100.0,
		ElevPenaltyBuffer: lcc.ElevPenaltyBuffer,
		ElevMaxBuffer:     lcc.ElevMaxBuffer,
		ElevBufferReduce:  lcc.ElevBufferReduce,
		TurnCost:          0, // no turn cost for landmarks
		Pass0Coefficient:  0, // unused in Dijkstra
		EdgeCost:          edgeCostFn,
	}
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)
	return cfg
}

// landmarkDijkstra runs a single-source Dijkstra over the entire graph via GraphReader,
// storing costs in a flat array indexed by DenseIndex. Returns costs for all reachable nodes.
func landmarkDijkstra(
	ctx context.Context,
	reader GraphReader,
	src uint64,
	cfg *CostConfig,
	idx *graph.DenseIndex,
) []float32 {
	n := idx.TotalNodes
	costs := make([]float32, n)
	for i := range costs {
		costs[i] = math.MaxFloat32
	}
	pathStates := make([]PathState, n)

	srcDense := idx.ToDense(src)
	if srcDense == math.MaxUint32 {
		slog.Warn("landmark dijkstra: source node not in dense index", "src", src)
		return costs
	}
	costs[srcDense] = 0

	frontier := &Frontier{}
	frontier.Push(FrontierEntry{NodeID: src, Cost: 0})

	expansions := 0
	const checkInterval = 8192
	const logInterval = checkInterval * 128 // ~1M

	for frontier.Len() > 0 {
		entry := frontier.Pop()
		nd := idx.ToDense(entry.NodeID)

		if entry.Cost > costs[nd]+0.001 {
			continue // stale entry
		}

		expansions++
		if expansions%checkInterval == 0 {
			if ctx.Err() != nil {
				break
			}
			if expansions%logInterval == 0 {
				slog.Info("landmark dijkstra progress",
					"expansions", expansions,
					"frontier", frontier.Len(),
				)
			}
		}

		reader.ForEachEdge(entry.NodeID, func(ev EdgeView) {
			ecf := cfg.EdgeCost(ev.Tags1, ev.Tags2)
			if ecf.Flat >= costBlocked {
				return
			}

			// Use min(Flat, Up, Down) to guarantee lower bound.
			// Hiking/MTB profiles can have Up < Flat (surfaceUphillMod = -0.1).
			cf := ecf.Flat
			if ecf.Up > 0 && ecf.Up < cf {
				cf = ecf.Up
			}
			if ecf.Down > 0 && ecf.Down < cf {
				cf = ecf.Down
			}
			distCost := float32(ev.Dist) / 100.0 * cf // Dist is in cm, convert to meters

			// Elevation cost via propagateHysteresis
			deltaEleMilli := ev.DeltaEle * 100   // dm -> mm
			distMilli := int32(ev.Dist) * 10      // cm -> mm
			hr := propagateHysteresis(pathStates[nd], deltaEleMilli, distMilli, cfg)

			newCost := costs[nd] + distCost + hr.ElevCost

			td := idx.ToDense(ev.Target)
			if td == math.MaxUint32 {
				return // target in unknown tile
			}
			if newCost < costs[td] {
				costs[td] = newCost
				pathStates[td] = hr.State
				frontier.Push(FrontierEntry{NodeID: ev.Target, Cost: newCost})
			}
		})
	}

	slog.Debug("landmark dijkstra complete",
		"src", src,
		"expansions", expansions,
	)

	return costs
}

// LandmarkPreprocessResult holds the per-set distance arrays for all nodes.
// Used as intermediate result before writing to tiles.
type LandmarkPreprocessResult struct {
	Meta *graph.LandmarkMetaData
	// PerSetDists[setID][landmarkIdx] = flat []float32 of length TotalNodes
	PerSetDists [NumLandmarkSets][LandmarksPerSet][]float32
}

// RunLandmarkPreprocessing orchestrates full landmark preprocessing:
//  1. Scan tiles + build DenseIndex
//  2. Select 16 landmarks (farthest uses Set A Dijkstra)
//  3. For each set SEQUENTIALLY: run 16 Dijkstras with errgroup.SetLimit(4),
//     store results, return for tile writing.
func RunLandmarkPreprocessing(
	ctx context.Context,
	reader GraphReader,
	tileDir string,
	progressFn func(step string, current, total int),
) (*LandmarkPreprocessResult, error) {
	t0 := time.Now()

	if progressFn == nil {
		progressFn = func(string, int, int) {}
	}

	// 1. Scan tiles
	progressFn("scanning tiles", 0, 0)
	tiles, err := graph.ScanTilesForLandmarks(tileDir)
	if err != nil {
		return nil, fmt.Errorf("scan tiles: %w", err)
	}
	if len(tiles) == 0 {
		return nil, fmt.Errorf("no tiles found in %s", tileDir)
	}

	idx := graph.BuildDenseIndex(tiles)
	slog.Info("landmark preprocessing: dense index built",
		"tiles", len(tiles),
		"totalNodes", idx.TotalNodes,
	)

	// 2. Select landmarks using Set A for farthest-point Dijkstra
	edgeCostFn := BuildLandmarkEdgeCostClosure()
	setACfg := BuildLandmarkCostConfig(LandmarkCostConfigs[LandmarkSetA], edgeCostFn)

	progressFn("selecting landmarks", 0, 16)
	farthestDijkstra := func(src uint64) (graph.LandmarkCandidate, error) {
		costs := landmarkDijkstra(ctx, reader, src, setACfg, idx)
		var best graph.LandmarkCandidate
		var bestCost float32
		for _, tile := range tiles {
			base, ok := idx.TileOffset[tile.TileID]
			if !ok {
				continue
			}
			for localID := range tile.NodeCount {
				cost := costs[base+localID]
				if cost > bestCost && cost < math.MaxFloat32 {
					bestCost = cost
					globalID := graph.MakeGlobalNodeID(tile.TileID, localID)
					lat, lon, ele := reader.NodeCoord(globalID)
					best = graph.LandmarkCandidate{
						GlobalNodeID: globalID,
						Lat:          lat,
						Lon:          lon,
						Ele:          ele,
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

	slog.Info("landmark preprocessing: landmarks selected",
		"count", meta.LandmarkCount,
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	result := &LandmarkPreprocessResult{Meta: meta}

	// 3. For each set SEQUENTIALLY: run 16 Dijkstras with errgroup.SetLimit(4)
	totalDijkstras := NumLandmarkSets * meta.LandmarkCount
	dijkstrasDone := 0

	for setIdx := range NumLandmarkSets {
		_ = LandmarkSetID(setIdx) // type-check only; setIdx used for indexing
		setCfg := BuildLandmarkCostConfig(LandmarkCostConfigs[setIdx], edgeCostFn)

		slog.Info("landmark preprocessing: starting set",
			"set", setIdx,
			"landmarks", meta.LandmarkCount,
		)

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(4)

		// Per-landmark cost arrays for this set
		setDists := make([][]float32, meta.LandmarkCount)

		for lk := range meta.LandmarkCount {
			g.Go(func() error {
				src := meta.NodeIDs[lk]
				if src == 0 {
					return nil
				}
				costs := landmarkDijkstra(gctx, reader, src, setCfg, idx)
				setDists[lk] = costs

				dijkstrasDone++
				progressFn("dijkstra runs", dijkstrasDone, totalDijkstras)
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, fmt.Errorf("landmark dijkstra set %d: %w", setIdx, err)
		}

		// Store in result
		for lk := range meta.LandmarkCount {
			result.PerSetDists[setIdx][lk] = setDists[lk]
		}

		slog.Info("landmark preprocessing: set complete",
			"set", setIdx,
			"elapsed", time.Since(t0).Round(time.Millisecond),
		)
	}

	meta.PreprocessingTime = time.Now()

	slog.Info("landmark preprocessing complete",
		"landmarks", meta.LandmarkCount,
		"sets", NumLandmarkSets,
		"totalDijkstras", totalDijkstras,
		"elapsed", time.Since(t0).Round(time.Second),
	)

	return result, nil
}

// PerTileLandmarkDists extracts per-tile distance arrays from the preprocessing result.
// Returns a map: tileID -> flat []float32 of length nodeCount * LandmarksPerSet.
// The layout is: for each node (0..nodeCount-1), 16 consecutive float32 distances.
// This is the format expected by graph.pendingLandmarkSections.
func (r *LandmarkPreprocessResult) PerTileLandmarkDists(
	setID LandmarkSetID,
	tiles []graph.TileScanResult,
	idx *graph.DenseIndex,
) map[uint32][]float32 {
	result := make(map[uint32][]float32, len(tiles))

	for _, tile := range tiles {
		nc := tile.NodeCount
		dists := make([]float32, nc*LandmarksPerSet)

		base, ok := idx.TileOffset[tile.TileID]
		if !ok {
			continue
		}

		for localID := range nc {
			denseID := base + localID
			for lk := range r.Meta.LandmarkCount {
				var cost float32
				if r.PerSetDists[setID][lk] != nil && denseID < uint32(len(r.PerSetDists[setID][lk])) {
					cost = r.PerSetDists[setID][lk][denseID]
				} else {
					cost = math.MaxFloat32
				}
				dists[localID*LandmarksPerSet+uint32(lk)] = cost
			}
			// Fill remaining landmark slots with MaxFloat32
			for lk := r.Meta.LandmarkCount; lk < LandmarksPerSet; lk++ {
				dists[localID*LandmarksPerSet+uint32(lk)] = math.MaxFloat32
			}
		}

		result[tile.TileID] = dists
	}

	return result
}
