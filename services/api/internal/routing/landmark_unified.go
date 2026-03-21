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
)

// landmarkDijkstraUnified runs a single-source Dijkstra on the unified flat CSR.
// No hysteresis (Set D: UphillCost=0, DownhillCost=0). No mutex, no DenseIndex.
func landmarkDijkstraUnified(ctx context.Context, csr *graph.UnifiedCSR, src uint32) []float32 {
	costs := make([]float32, csr.NodeCount)
	for i := range costs {
		costs[i] = math.MaxFloat32
	}
	costs[src] = 0

	// Pre-allocate frontier to reduce GC pressure during full-graph Dijkstra
	frontier := &Frontier{}
	frontier.Reset() // ensure size=0
	// entries will grow via append — pre-allocation not critical with lazy deletion
	frontier.Push(FrontierEntry{NodeID: uint64(src), Cost: 0})

	expansions := 0
	for frontier.Len() > 0 {
		entry := frontier.Pop()
		nd := uint32(entry.NodeID) // Safe: denseID always < uint32 max (NodeCount is uint32)
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
func checkpointPath(tileDir string, lk int) string {
	return filepath.Join(tileDir, fmt.Sprintf("landmark_checkpoint_%02d.bin", lk))
}

// saveDijkstraCheckpoint writes a costs array to disk as a binary checkpoint.
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
	return os.Rename(f.Name(), path) // atomic
}

// loadDijkstraCheckpoint reads a costs array from a binary checkpoint.
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

// cleanupCheckpoints removes all landmark checkpoint files.
func cleanupCheckpoints(tileDir string) {
	matches, _ := filepath.Glob(filepath.Join(tileDir, "landmark_checkpoint_*.bin"))
	for _, m := range matches {
		os.Remove(m)
	}
}

// RunUnifiedLandmarkPreprocessing computes landmarks using Set D only (UphillCost=0,
// DownhillCost=0). This is admissible for ALL profiles but provides a loose bound
// for elevation-heavy profiles (Hiking/MTB). Profiles selecting Set A/B/C will
// degrade to LowerBound()=0 (euclidean-only heuristic) until elevation-aware sets
// are implemented via unified CSR. Route quality is unaffected — only search speed.
//
// Checkpoints: Each Dijkstra result is saved to disk immediately after completion.
// On crash/restart, existing checkpoints are loaded instead of re-computed.
// CSR is freed BEFORE the write-back phase to reduce peak memory.
func RunUnifiedLandmarkPreprocessing(
	ctx context.Context,
	tileDir string,
	progressFn func(step string, current, total int),
) (*graph.LandmarkMetaData, error) {
	if progressFn == nil {
		progressFn = func(string, int, int) {}
	}
	t0 := time.Now()

	// 1. Build unified CSR from all tiles.
	progressFn("building unified CSR", 0, 0)
	csr, nodeMapping, tiles, err := graph.BuildUnifiedCSR(ctx, tileDir, BuildLandmarkEdgeCostClosureCSR(), progressFn)
	if err != nil {
		return nil, fmt.Errorf("build unified CSR: %w", err)
	}

	slog.Info("unified landmark preprocessing: CSR built",
		"nodes", csr.NodeCount,
		"edges", csr.EdgeCount,
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	// 2. Select landmarks with farthestDijkstra closure
	progressFn("selecting landmarks", 0, 16)
	farthestDijkstra := func(src uint64) (graph.LandmarkCandidate, error) {
		srcTileID, srcLocalID := graph.SplitGlobalNodeID(src)
		mapping := nodeMapping[srcTileID]
		if mapping == nil || uint32(len(mapping)) <= srcLocalID {
			return graph.LandmarkCandidate{}, fmt.Errorf("node %d not in CSR mapping (tile %d, local %d)", src, srcTileID, srcLocalID)
		}
		srcDense := mapping[srcLocalID]
		costs := landmarkDijkstraUnified(ctx, csr, srcDense)
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

	// 3. Run 16 Dijkstras with errgroup.SetLimit(2)
	// Each result is checkpointed to disk immediately.
	// On resume: existing checkpoints are loaded instead of re-computed.
	var dijkstrasDone atomic.Int64
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(2)

	for lk := range meta.LandmarkCount {
		eg.Go(func() error {
			ckptPath := checkpointPath(tileDir, lk)

			// Resume: load existing checkpoint if valid
			if existing, loadErr := loadDijkstraCheckpoint(ckptPath, csr.NodeCount); loadErr == nil {
				done := dijkstrasDone.Add(1)
				slog.Info("landmark dijkstra loaded from checkpoint",
					"landmark", lk, "run", done, "total", meta.LandmarkCount)
				progressFn("dijkstra runs", int(done), meta.LandmarkCount)
				// Save to a temporary in-memory reference — will be re-loaded from checkpoint during write-back
				_ = existing
				return nil
			}

			tileID, localID := graph.SplitGlobalNodeID(meta.NodeIDs[lk])
			mapping := nodeMapping[tileID]
			if mapping == nil || uint32(len(mapping)) <= localID {
				return fmt.Errorf("landmark %d node not in CSR mapping (tile %d, local %d)", lk, tileID, localID)
			}
			srcDense := mapping[localID]
			costs := landmarkDijkstraUnified(egCtx, csr, srcDense)

			// Checkpoint to disk immediately (atomic write)
			if saveErr := saveDijkstraCheckpoint(ckptPath, costs); saveErr != nil {
				slog.Warn("failed to save landmark checkpoint", "landmark", lk, "error", saveErr)
				// Non-fatal: we still have costs in memory for this run
			}

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

	// 4. FREE CSR before write-back to reduce peak memory.
	// The Dijkstra results are on disk as checkpoints.
	// We reload them one-at-a-time during write-back.
	nodeCount := csr.NodeCount
	csr.Close()
	csr = nil
	debug.FreeOSMemory()

	slog.Info("unified landmark preprocessing: CSR freed, starting write-back")

	// 5. Write back to tiles — load checkpoints one-at-a-time
	progressFn("writing landmark distances", 0, len(tiles))

	// Load ALL checkpoints (16 × 191 MB = ~3 GB — fits after CSR freed)
	allCosts := make([][]float32, meta.LandmarkCount)
	for lk := range meta.LandmarkCount {
		ckptPath := checkpointPath(tileDir, lk)
		costs, loadErr := loadDijkstraCheckpoint(ckptPath, nodeCount)
		if loadErr != nil {
			return nil, fmt.Errorf("load landmark checkpoint %d: %w", lk, loadErr)
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

	// Free allCosts before tile serialization
	allCosts = nil
	debug.FreeOSMemory()

	if err := graph.WriteLandmarkSectionsForSet(tileDir, tiles, meta, graph.SectionLandmarkDistD, tileDistArrays); err != nil {
		return nil, fmt.Errorf("write landmark sections: %w", err)
	}

	// 6. Cleanup checkpoints (success — no longer needed)
	cleanupCheckpoints(tileDir)

	slog.Info("unified landmark preprocessing complete",
		"landmarks", meta.LandmarkCount,
		"tiles", len(tiles),
		"totalElapsed", time.Since(t0).Round(time.Second),
	)

	return meta, nil
}
