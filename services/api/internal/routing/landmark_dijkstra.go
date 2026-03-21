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

// RunLandmarkPreprocessing orchestrates full landmark preprocessing:
//  1. Scan tiles + build DenseIndex
//  2. Select 16 landmarks (farthest uses Set A Dijkstra)
//  3. For each set SEQUENTIALLY: run 16 Dijkstras with dynamic parallelism,
//     write landmark sections to tiles, FREE arrays, next set.
//
// Peak memory: ~2.3 GB (1 set × 16 landmarks × totalNodes) instead of ~12 GB.
func RunLandmarkPreprocessing(
	ctx context.Context,
	reader GraphReader,
	tileDir string,
	progressFn func(step string, current, total int),
) (*graph.LandmarkMetaData, error) {
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

	// 3. For each set SEQUENTIALLY: run Dijkstras, write to tiles, FREE, next set.
	totalDijkstras := NumLandmarkSets * meta.LandmarkCount
	var dijkstrasDone atomic.Int64

	// W4: Dynamic parallelism based on memory budget.
	// Each Dijkstra needs ~12 bytes per node (costs float32 + pathStates 8 bytes).
	bytesPerRun := int64(idx.TotalNodes) * 12
	maxParallel := 4
	if bytesPerRun > 0 {
		// Target max 3 GB for parallel Dijkstras (leave room for tile cache + other).
		maxByBudget := int(3_000_000_000 / bytesPerRun)
		maxParallel = max(1, min(4, maxByBudget))
	}
	slog.Info("landmark dijkstra parallelism", "bytesPerRun", bytesPerRun, "maxParallel", maxParallel)

	for setIdx := range NumLandmarkSets {
		setCfg := BuildLandmarkCostConfig(LandmarkCostConfigs[setIdx], edgeCostFn)

		slog.Info("landmark preprocessing: starting set",
			"set", setIdx,
			"landmarks", meta.LandmarkCount,
		)

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(maxParallel)

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

				done := dijkstrasDone.Add(1)
				progressFn("dijkstra runs", int(done), totalDijkstras)
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, fmt.Errorf("landmark dijkstra set %d: %w", setIdx, err)
		}

		// K2: Write landmark sections for this set immediately, then free arrays.
		tileDistArrays := make(map[uint32][]float32, len(tiles))
		for _, tile := range tiles {
			dists := make([]float32, int(tile.NodeCount)*LandmarksPerSet)
			for i := range dists {
				dists[i] = math.MaxFloat32
			}
			base, ok := idx.TileOffset[tile.TileID]
			if !ok {
				continue
			}
			for lk := range meta.LandmarkCount {
				if setDists[lk] == nil {
					continue
				}
				for localID := range tile.NodeCount {
					dists[localID*LandmarksPerSet+uint32(lk)] = setDists[lk][base+localID]
				}
			}
			tileDistArrays[tile.TileID] = dists
		}

		sectionID := graph.SectionLandmarkDistA + uint16(setIdx)
		if writeErr := graph.WriteLandmarkSectionsForSet(tileDir, tiles, meta, sectionID, tileDistArrays); writeErr != nil {
			return nil, fmt.Errorf("write landmark sections set %d: %w", setIdx, writeErr)
		}

		// Free per-set arrays immediately.
		for lk := range meta.LandmarkCount {
			setDists[lk] = nil
		}
		tileDistArrays = nil

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

	return meta, nil
}

