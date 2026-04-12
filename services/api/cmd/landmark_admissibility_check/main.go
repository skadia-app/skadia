// landmark_admissibility_check is a POC tool that verifies the ALT lower-bound
// property for landmark edge costs across all routing profiles.
//
// For 10,000 randomly sampled edges it compares landmark_edge_cost.Flat
// vs profile_edge_cost.Flat and reports violations (landmark > profile).
// Admissibility requires: landmark.Flat <= profile.Flat for all non-blocked edges.
//
// Usage:
//
//	landmark_admissibility_check -tile-dir /data/routing/tiles -profile-dir /config/badrouter
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

const (
	targetEdges      = 10_000
	costBlocked      = float32(10_000)
	maxNodesPerTile  = 5_000_000 // safety cap — tiles never exceed this
	maxAttemptsRatio = 10        // try up to 10x targetEdges node probes total
)

// profileNames lists all built-in routing profiles to check.
var profileNames = []string{"hiking", "mtb", "gravel", "trekking", "road"}

// edgeSample records the cost factors for one sampled edge.
type edgeSample struct {
	tags1, tags2    uint64
	landmarkFlat    float32
	profileFlat     float32
}

// profileStats accumulates per-profile admissibility statistics.
type profileStats struct {
	name        string
	total       int
	violations  int
	maxGap      float64 // max (landmark - profile) / profile
	gapSamples  []float64
}

func main() {
	tileDir := flag.String("tile-dir", "/data/routing/tiles", "Directory containing tile_*.bin files")
	profileDir := flag.String("profile-dir", "/config/badrouter", "Directory containing profile *.json files")
	seed := flag.Uint64("seed", 42, "Random seed for reproducible sampling")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Load profiles ───────────────────────────────────────────────────────
	n, err := routing.LoadBuiltinProfiles(*profileDir)
	if err != nil {
		slog.Error("LoadBuiltinProfiles failed", "err", err)
		os.Exit(1)
	}
	slog.Info("profiles loaded", "count", n, "dir", *profileDir)

	// ── Build cost closures ─────────────────────────────────────────────────
	landmarkClosure := routing.BuildLandmarkEdgeCostClosure()

	type profileClosure struct {
		name string
		fn   func(tags1, tags2 uint64) routing.EdgeCostFactors
	}

	var closures []profileClosure
	for _, name := range profileNames {
		cfg, err := routing.BuildCostConfig(routing.ProfileConfig{Profile: name})
		if err != nil {
			slog.Error("BuildCostConfig failed", "profile", name, "err", err)
			os.Exit(1)
		}
		closures = append(closures, profileClosure{name: name, fn: cfg.EdgeCost})
	}

	// ── Enumerate tiles ─────────────────────────────────────────────────────
	tileIDs, err := listTileIDs(*tileDir)
	if err != nil {
		slog.Error("listTileIDs failed", "err", err)
		os.Exit(1)
	}
	if len(tileIDs) == 0 {
		slog.Error("no tiles found", "dir", *tileDir)
		os.Exit(1)
	}
	slog.Info("tiles found", "count", len(tileIDs))

	// ── Create graph reader ─────────────────────────────────────────────────
	const cacheBudget = 4 * 1024 * 1024 * 1024 // 4 GB — generous for sampling
	reader := routing.NewTiledGraphReader(*tileDir, cacheBudget)

	// ── Sample edges ────────────────────────────────────────────────────────
	rng := rand.New(rand.NewPCG(uint64(*seed), 0))

	samples := make([]edgeSample, 0, targetEdges)
	maxAttempts := targetEdges * maxAttemptsRatio

	// Shuffle tile list for random coverage.
	shuffledTiles := make([]uint32, len(tileIDs))
	copy(shuffledTiles, tileIDs)
	rng.Shuffle(len(shuffledTiles), func(i, j int) {
		shuffledTiles[i], shuffledTiles[j] = shuffledTiles[j], shuffledTiles[i]
	})

	attempts := 0
	tileIdx := 0

	for len(samples) < targetEdges && attempts < maxAttempts {
		// Cycle through tiles if we run out.
		if tileIdx >= len(shuffledTiles) {
			tileIdx = 0
		}
		tileID := shuffledTiles[tileIdx]
		tileIdx++

		// Load graph for this tile to get its NodeCount.
		lat, lon := tileCenterCoord(tileID)
		g := reader.GraphAt(lat, lon)
		if g == nil || g.NodeCount == 0 {
			attempts++
			continue
		}

		// Try a random local node from this tile.
		localID := rng.Uint32N(g.NodeCount)
		nodeID := routing.MakeGlobalNodeID(tileID, localID)

		reader.ForEachEdge(nodeID, func(ev routing.EdgeView) {
			if len(samples) >= targetEdges {
				return
			}
			lmf := landmarkClosure(ev.Tags1, ev.Tags2)
			// Skip blocked edges — admissibility only applies to traversable edges.
			if lmf.Flat >= costBlocked {
				return
			}
			samples = append(samples, edgeSample{
				tags1:        ev.Tags1,
				tags2:        ev.Tags2,
				landmarkFlat: lmf.Flat,
			})
		})
		attempts++
	}

	if len(samples) == 0 {
		slog.Error("no edges sampled — check tile-dir and that tiles are populated")
		os.Exit(1)
	}
	slog.Info("edges sampled", "count", len(samples), "attempts", attempts)

	// ── Evaluate per profile ────────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("%-12s  %-8s  %-10s  %-12s  %-12s  %-10s\n",
		"PROFILE", "EDGES", "VIOLATIONS", "VIOL_RATE", "MAX_GAP", "MEDIAN_GAP")
	fmt.Println(strings.Repeat("-", 74))

	allPassed := true

	for _, cl := range closures {
		stats := computeStats(cl.name, samples, cl.fn)

		violRate := float64(stats.violations) / float64(stats.total) * 100.0
		medianGap := median(stats.gapSamples)

		maxGapPct := stats.maxGap * 100.0
		medianGapPct := medianGap * 100.0

		passIcon := "OK"
		if violRate > 1.0 || maxGapPct > 5.0 {
			passIcon = "FAIL"
			allPassed = false
		}

		fmt.Printf("%-12s  %-8d  %-10d  %8.4f%%    %8.4f%%    %8.4f%%  %s\n",
			cl.name,
			stats.total,
			stats.violations,
			violRate,
			maxGapPct,
			medianGapPct,
			passIcon,
		)
	}

	fmt.Println()
	if allPassed {
		fmt.Println("RESULT: PASSED — all profiles satisfy admissibility constraints")
		fmt.Println("  (violation rate <= 1%, max gap <= 5%)")
	} else {
		fmt.Println("RESULT: FAILED — one or more profiles violated admissibility constraints")
		fmt.Println("  (required: violation rate <= 1%, max gap <= 5%)")
		os.Exit(1)
	}
}

// computeStats evaluates admissibility for a profile against the collected samples.
func computeStats(name string, samples []edgeSample, profileCost func(tags1, tags2 uint64) routing.EdgeCostFactors) profileStats {
	stats := profileStats{name: name}

	for i := range samples {
		s := &samples[i]
		pcf := profileCost(s.tags1, s.tags2)

		// Profile may block this edge (e.g. road profile blocks footpaths).
		// A blocked profile edge is trivially admissible since no path will use it.
		if pcf.Flat >= costBlocked {
			continue
		}

		stats.total++

		// Admissibility: landmark.Flat <= profile.Flat
		// (landmark cost must be a lower bound of the real profile cost)
		if s.landmarkFlat > pcf.Flat {
			stats.violations++
			gap := float64(s.landmarkFlat-pcf.Flat) / float64(pcf.Flat)
			if gap > stats.maxGap {
				stats.maxGap = gap
			}
			stats.gapSamples = append(stats.gapSamples, gap)
		}
	}

	return stats
}

// listTileIDs reads the tile directory and returns all valid tile IDs.
func listTileIDs(dir string) ([]uint32, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("ReadDir %q: %w", dir, err)
	}

	var ids []uint32
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "tile_") || !strings.HasSuffix(e.Name(), ".bin") {
			continue
		}
		// Parse "tile_{lat}_{lon}.bin"
		base := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "tile_"), ".bin")
		parts := strings.Split(base, "_")
		if len(parts) != 2 {
			continue
		}
		lat, err1 := strconv.Atoi(parts[0])
		lon, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}
		tileID := graph.TileIDFromCoord(float64(lat), float64(lon))
		ids = append(ids, tileID)
	}

	if len(ids) == 0 {
		// Try glob as fallback (handles symlinks etc.)
		matches, _ := filepath.Glob(filepath.Join(dir, "tile_*.bin"))
		for _, m := range matches {
			base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "tile_"), ".bin")
			parts := strings.Split(base, "_")
			if len(parts) != 2 {
				continue
			}
			lat, err1 := strconv.Atoi(parts[0])
			lon, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil {
				continue
			}
			ids = append(ids, graph.TileIDFromCoord(float64(lat), float64(lon)))
		}
	}

	return ids, nil
}

// tileCenterCoord returns the center lat/lon of the 1°x1° tile.
func tileCenterCoord(tileID uint32) (lat, lon float64) {
	latFloor := int(tileID/360) - 90
	lonFloor := int(tileID%360) - 180
	return float64(latFloor) + 0.5, float64(lonFloor) + 0.5
}

// median returns the median of a float64 slice. Returns 0 for empty slices.
func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	return sorted[n/2]
}

// _ prevents "imported and not used" if math is only used via math.Floor indirectly.
var _ = math.Floor
