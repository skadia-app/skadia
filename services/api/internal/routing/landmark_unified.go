package routing

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/jackc/pgx/v5/pgxpool"
)

// landmarkDijkstraUnified runs a single-source Dijkstra on the unified flat CSR.
// For Set D (UphillCost=0): no hysteresis (pure flat distance).
// For Sets A/B/C: includes propagateHysteresis using DeltaEle from edges.
func landmarkDijkstraUnified(ctx context.Context, csr *graph.UnifiedCSR, src uint32, cfg *CostConfig) []float32 {
	costs := make([]float32, csr.NodeCount)
	for i := range costs {
		costs[i] = math.MaxFloat32
	}
	costs[src] = 0

	useHysteresis := cfg.UphillCost > 0 || cfg.DownhillCost > 0
	var pathStates []PathState
	if useHysteresis {
		pathStates = make([]PathState, csr.NodeCount)
	}

	frontier := &Frontier{}
	frontier.Push(FrontierEntry{NodeID: uint64(src), Cost: 0})

	expansions := 0
	for frontier.Len() > 0 {
		entry := frontier.Pop()
		nd := uint32(entry.NodeID)
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

			distCost := float32(edge.Dist) / 100.0 * edge.CostFactor
			var elevCost float32

			if useHysteresis {
				deltaEleMilli := int32(edge.DeltaEle) * 100 // dm → mm
				distMilli := int32(edge.Dist) * 10           // cm → mm
				hr := propagateHysteresis(pathStates[nd], deltaEleMilli, distMilli, cfg)
				elevCost = hr.ElevCost

				newCost := costs[nd] + distCost + elevCost
				if newCost < costs[edge.Target] {
					costs[edge.Target] = newCost
					pathStates[edge.Target] = hr.State
					frontier.Push(FrontierEntry{NodeID: uint64(edge.Target), Cost: newCost})
				}
			} else {
				newCost := costs[nd] + distCost
				if newCost < costs[edge.Target] {
					costs[edge.Target] = newCost
					frontier.Push(FrontierEntry{NodeID: uint64(edge.Target), Cost: newCost})
				}
			}
		}
	}

	slog.Info("landmark dijkstra complete", "expansions", expansions)
	return costs
}

// landmarkBudgetDijkstraUnified runs a budget-limited Dijkstra on the unified CSR.
// Stops when all remaining frontier entries exceed maxCost.
// Used for crossing-flank detection (50km budget).
func landmarkBudgetDijkstraUnified(ctx context.Context, csr *graph.UnifiedCSR, src uint32, cfg *CostConfig, maxCost float32) []float32 {
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
		if entry.Cost > maxCost {
			break // min-heap: all remaining entries exceed budget
		}
		nd := uint32(entry.NodeID)
		if entry.Cost > costs[nd]+0.001 {
			continue
		}

		expansions++
		if expansions%1_000_000 == 0 && ctx.Err() != nil {
			break
		}

		for ei := csr.Offsets[nd]; ei < csr.Offsets[nd+1]; ei++ {
			edge := csr.Edges[ei]
			if edge.CostFactor >= costBlocked {
				continue
			}
			distCost := float32(edge.Dist) / 100.0 * edge.CostFactor
			newCost := costs[nd] + distCost
			if newCost < costs[edge.Target] && newCost <= maxCost {
				costs[edge.Target] = newCost
				frontier.Push(FrontierEntry{NodeID: uint64(edge.Target), Cost: newCost})
			}
		}
	}

	slog.Info("budget dijkstra complete", "expansions", expansions, "budget", maxCost)
	return costs
}

// BuildLandmarkEdgeCostClosureCSR adapts the routing package's EdgeCost closure
// to the graph package's EdgeCostFunc type.
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

// checkpointPath returns the file path for a landmark Dijkstra checkpoint.
// Format: landmark_checkpoint_S{set}_L{landmark}.bin
func checkpointPath(tileDir string, setIdx, lk int) string {
	return filepath.Join(tileDir, fmt.Sprintf("landmark_checkpoint_S%d_L%02d.bin", setIdx, lk))
}

func saveDijkstraCheckpoint(path string, costs []float32) error {
	f, err := os.CreateTemp(filepath.Dir(path), "lm_ckpt_*.tmp")
	if err != nil {
		return err
	}
	buf := make([]byte, len(costs)*4)
	for i, c := range costs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(c))
	}
	if _, err := f.Write(buf); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	f.Close()
	return os.Rename(f.Name(), path)
}

func loadDijkstraCheckpoint(path string, nodeCount uint32) ([]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	expected := int(nodeCount) * 4
	if len(data) != expected {
		return nil, fmt.Errorf("checkpoint size mismatch: got %d, want %d", len(data), expected)
	}
	costs := make([]float32, nodeCount)
	for i := range costs {
		costs[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return costs, nil
}

func cleanupCheckpoints(tileDir string) {
	matches, _ := filepath.Glob(filepath.Join(tileDir, "landmark_checkpoint_*.bin"))
	for _, m := range matches {
		os.Remove(m)
	}
}

// RunUnifiedLandmarkPreprocessing computes landmarks for ALL 4 cost-config sets
// (A=7/7, B=7/0, C=0/7, D=0/0). Each set has 16 landmarks = 64 Dijkstras total.
// Sets are processed SEQUENTIALLY to limit memory. Within each set, 2 Dijkstras
// run in parallel. Each result is checkpointed to disk for crash-recovery.
// CSR is freed BEFORE write-back to reduce peak memory.
//
// With crossing-flanks strategy (default): queries OSM crossings via pool,
// runs budget Dijkstras, places flank landmarks.
// pool is required for OSM crossing queries. Pass nil to skip crossing detection
// (falls back to legacy BBox+Farthest strategy).
func RunUnifiedLandmarkPreprocessing(
	ctx context.Context,
	tileDir string,
	pool *pgxpool.Pool, // for OSM crossing queries (nil = legacy mode)
	progressFn func(step string, current, total int),
) (*graph.LandmarkMetaData, error) {
	if progressFn == nil {
		progressFn = func(string, int, int) {}
	}
	t0 := time.Now()

	// 1. Build unified CSR
	progressFn("building unified CSR", 0, 0)
	csr, nodeMapping, tiles, err := graph.BuildUnifiedCSR(ctx, tileDir, BuildLandmarkEdgeCostClosureCSR(), progressFn)
	if err != nil {
		return nil, fmt.Errorf("build unified CSR: %w", err)
	}

	slog.Info("unified landmark preprocessing: CSR built",
		"nodes", csr.NodeCount, "edges", csr.EdgeCount,
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	// 2. Select landmarks
	progressFn("selecting landmarks", 0, 16)

	// 2a. Detect crossing flanks (if DB pool available and not legacy mode)
	var flankLandmarks []graph.LandmarkCandidate

	if pool != nil && os.Getenv("LANDMARK_STRATEGY") != "legacy" {
		south, west, north, east := BBoxFromTiles(tiles)

		// SQL returns top-20 per quadrant (up to 80 candidates), ordered by ele DESC.
		// We snap to graph, filter by CSR degree, and pick 1 per quadrant.
		seeds, seedErr := SelectCrossingSeedsSQL(ctx, pool, west, south, east, north)
		if seedErr != nil {
			slog.Warn("crossing seed query failed, falling back", "error", seedErr)
		} else if len(seeds) > 0 {
			slog.Info("crossing seed candidates from DB", "count", len(seeds))
			snappedSeeds := SnapOSMLandmarksToGraph(seeds, tileDir)

			// Phase A: Filter by CSR degree >= 3 (O(1) per candidate).
			// Glacial saddles snap to degree 0-2 nodes. Trail passes have degree >= 3.
			// Seeds are already quadrant-ordered from SQL, so first connected candidate
			// per quadrant wins (highest elevation among connected ones).
			midLat := (south + north) / 2
			midLon := (west + east) / 2

			type quadrantSeed struct {
				osmIdx  int
				denseID uint32
				degree  uint32
			}
			quadrantBest := make(map[[2]int]quadrantSeed)

			for si, seed := range snappedSeeds {
				if si >= len(seeds) {
					break
				}
				seedTileID, seedLocalID := graph.SplitGlobalNodeID(seed.GlobalNodeID)
				mapping := nodeMapping[seedTileID]
				if mapping == nil || uint32(len(mapping)) <= seedLocalID {
					continue
				}
				denseID := mapping[seedLocalID]
				degree := csr.Offsets[denseID+1] - csr.Offsets[denseID]
				if degree < 3 {
					continue
				}

				quadrant := [2]int{0, 0}
				if seeds[si].Lat >= midLat { quadrant[0] = 1 }
				if seeds[si].Lon >= midLon { quadrant[1] = 1 }

				// First connected candidate per quadrant wins (SQL ordered by ele DESC)
				if _, ok := quadrantBest[quadrant]; !ok {
					quadrantBest[quadrant] = quadrantSeed{si, denseID, degree}
				}
			}

			slog.Info("crossing seeds after connectivity filter",
				"quadrants_with_seeds", len(quadrantBest))

			// Phase B: Budget-Dijkstra + flank detection (max 4 runs).
			setCfg := BuildLandmarkCostConfig(LandmarkCostConfigs[LandmarkSetD], BuildLandmarkEdgeCostClosure())
			seedNum := 0

			for quadrant, qs := range quadrantBest {
				seedNum++
				progressFn(fmt.Sprintf("flank dijkstra %d/%d", seedNum, len(quadrantBest)), seedNum-1, len(quadrantBest))
				costs := landmarkBudgetDijkstraUnified(ctx, csr, qs.denseID, setCfg, 50000)

				reachable := 0
				for _, c := range costs {
					if c < math.MaxFloat32 { reachable++ }
				}
				if reachable < 5000 {
					slog.Warn("crossing seed poorly connected, skipping",
						"name", seeds[qs.osmIdx].Name, "ele", seeds[qs.osmIdx].Ele,
						"degree", qs.degree, "reachable", reachable, "quadrant", quadrant)
					continue
				}

				slog.Info("crossing seed accepted",
					"name", seeds[qs.osmIdx].Name, "ele", seeds[qs.osmIdx].Ele,
					"degree", qs.degree, "reachable", reachable, "quadrant", quadrant)

				seed := snappedSeeds[qs.osmIdx]
				flankA, flankB := FindFlankLandmarks(seed, costs, tiles, nodeMapping)
				if flankA.GlobalNodeID != 0 {
					flankLandmarks = append(flankLandmarks, flankA)
					slog.Info("flank landmark found",
						"seed_name", seeds[qs.osmIdx].Name, "side", "A",
						"lat", float64(flankA.Lat)/1e7, "lon", float64(flankA.Lon)/1e7,
						"ele_m", float64(flankA.Ele)/10.0)
				}
				if flankB.GlobalNodeID != 0 {
					flankLandmarks = append(flankLandmarks, flankB)
					slog.Info("flank landmark found",
						"seed_name", seeds[qs.osmIdx].Name, "side", "B",
						"lat", float64(flankB.Lat)/1e7, "lon", float64(flankB.Lon)/1e7,
						"ele_m", float64(flankB.Ele)/10.0)
				}
			}
			slog.Info("crossing seeds used", "accepted", seedNum, "candidates", len(seeds))
		}
	}

	// 2b. Farthest Dijkstra callback (uses Set D flat distance)
	farthestDijkstra := func(src uint64) (graph.LandmarkCandidate, error) {
		srcTileID, srcLocalID := graph.SplitGlobalNodeID(src)
		mapping := nodeMapping[srcTileID]
		if mapping == nil || uint32(len(mapping)) <= srcLocalID {
			return graph.LandmarkCandidate{}, fmt.Errorf("node %d not in mapping", src)
		}
		setCfg := BuildLandmarkCostConfig(LandmarkCostConfigs[LandmarkSetD], BuildLandmarkEdgeCostClosure())
		costs := landmarkDijkstraUnified(ctx, csr, mapping[srcLocalID], setCfg)
		var best graph.LandmarkCandidate
		var bestCost float32
		for _, tile := range tiles {
			tm := nodeMapping[tile.TileID]
			if tm == nil {
				continue
			}
			for localID := range tile.NodeCount {
				if localID >= uint32(len(tm)) {
					continue
				}
				cost := costs[tm[localID]]
				if cost > bestCost && cost < math.MaxFloat32 {
					bestCost = cost
					best = graph.LandmarkCandidate{
						GlobalNodeID: graph.MakeGlobalNodeID(tile.TileID, localID),
						Lat: tile.NodeLats[localID], Lon: tile.NodeLons[localID], Ele: tile.NodeEles[localID],
					}
				}
			}
		}
		return best, nil
	}

	// 2c. Select landmarks with new or legacy strategy
	var meta *graph.LandmarkMetaData
	if os.Getenv("LANDMARK_STRATEGY") == "legacy" || pool == nil {
		var osmPassC []graph.LandmarkCandidate
		if pool != nil {
			south, west, north, east := BBoxFromTiles(tiles)
			osmPass, _ := QueryOSMPassLandmarks(ctx, pool, west, south, east, north, 4)
			osmPassC = SnapOSMLandmarksToGraph(osmPass, tileDir)
		}
		meta, err = graph.SelectLandmarksLegacy(tiles, farthestDijkstra, osmPassC, nil)
	} else {
		meta, err = graph.SelectLandmarks(tiles, flankLandmarks, farthestDijkstra)
	}
	if err != nil {
		return nil, fmt.Errorf("select landmarks: %w", err)
	}

	slog.Info("unified landmark preprocessing: landmarks selected",
		"count", meta.LandmarkCount,
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	totalDijkstras := NumLandmarkSets * meta.LandmarkCount
	var globalDone atomic.Int64

	// 3. Process each set SEQUENTIALLY
	for setIdx := range NumLandmarkSets {
		lcfg := LandmarkCostConfigs[setIdx]
		setCfg := BuildLandmarkCostConfig(lcfg, BuildLandmarkEdgeCostClosure())
		sectionID := graph.SectionLandmarkDistA + uint16(setIdx)

		slog.Info("landmark set starting",
			"set", setIdx, "uphill", lcfg.UphillCost, "downhill", lcfg.DownhillCost)

		// 3a. Run 16 Dijkstras (2 parallel) with checkpoints
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(2)

		for lk := range meta.LandmarkCount {
			eg.Go(func() error {
				ckptPath := checkpointPath(tileDir, setIdx, lk)
				if _, loadErr := loadDijkstraCheckpoint(ckptPath, csr.NodeCount); loadErr == nil {
					done := globalDone.Add(1)
					slog.Info("landmark dijkstra resumed from checkpoint",
						"set", setIdx, "landmark", lk, "run", done, "total", totalDijkstras)
					progressFn("dijkstra runs", int(done), totalDijkstras)
					return nil
				}

				srcTileID, srcLocalID := graph.SplitGlobalNodeID(meta.NodeIDs[lk])
				mapping := nodeMapping[srcTileID]
				if mapping == nil || uint32(len(mapping)) <= srcLocalID {
					return fmt.Errorf("set %d landmark %d: node not in mapping", setIdx, lk)
				}
				costs := landmarkDijkstraUnified(egCtx, csr, mapping[srcLocalID], setCfg)

				if saveErr := saveDijkstraCheckpoint(ckptPath, costs); saveErr != nil {
					slog.Warn("checkpoint save failed", "set", setIdx, "landmark", lk, "error", saveErr)
				}

				done := globalDone.Add(1)
				slog.Info("landmark dijkstra complete",
					"set", setIdx, "landmark", lk, "run", done, "total", totalDijkstras)
				progressFn("dijkstra runs", int(done), totalDijkstras)
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return nil, fmt.Errorf("dijkstras set %d: %w", setIdx, err)
		}

		slog.Info("landmark set dijkstras complete", "set", setIdx,
			"elapsed", time.Since(t0).Round(time.Second))

		// 3b. Load checkpoints + write to tiles for THIS set
		// (CSR still needed for nodeMapping, freed after ALL sets)
		allCosts := make([][]float32, meta.LandmarkCount)
		for lk := range meta.LandmarkCount {
			ckptPath := checkpointPath(tileDir, setIdx, lk)
			costs, loadErr := loadDijkstraCheckpoint(ckptPath, csr.NodeCount)
			if loadErr != nil {
				return nil, fmt.Errorf("load checkpoint set %d lm %d: %w", setIdx, lk, loadErr)
			}
			allCosts[lk] = costs
		}

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

		// Free allCosts before tile write
		allCosts = nil

		progressFn(fmt.Sprintf("writing set %d", setIdx), setIdx, NumLandmarkSets)
		if err := graph.WriteLandmarkSectionsForSet(tileDir, tiles, meta, sectionID, tileDistArrays); err != nil {
			return nil, fmt.Errorf("write sections set %d: %w", setIdx, err)
		}
		tileDistArrays = nil
		debug.FreeOSMemory()

		slog.Info("landmark set written", "set", setIdx,
			"elapsed", time.Since(t0).Round(time.Second))
	}

	// 4. Cleanup
	csr.Close()
	cleanupCheckpoints(tileDir)

	slog.Info("unified landmark preprocessing complete",
		"landmarks", meta.LandmarkCount, "sets", NumLandmarkSets,
		"tiles", len(tiles),
		"totalElapsed", time.Since(t0).Round(time.Second),
	)
	return meta, nil
}
