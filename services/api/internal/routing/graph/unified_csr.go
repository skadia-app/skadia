package graph

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

// EdgeCostFactors holds pre-computed cost multipliers for an edge.
// Flat is used for flat terrain; Up/Down are for uphill/downhill (0 = use Flat).
type EdgeCostFactors struct {
	Flat float32
	Up   float32
	Down float32
}

// PrecomputedEdge stores pre-computed edge data for flat-CSR Dijkstra.
// 16 bytes (aligned). CostFactor = min(Flat, Up>0?Up:Flat, Down>0?Down:Flat).
type PrecomputedEdge struct {
	Target     uint32  // Dense Node-ID
	Dist       uint32  // Distance in cm (uint32 — edges >655m exist in outdoor graphs)
	CostFactor float32 // Pre-computed min costfactor
	_          uint32  // Padding (reserved for DeltaEle in 4-Set follow-up)
}

// UnifiedCSR is a temporary flat graph for landmark Dijkstra preprocessing.
// Built from all tiles, used for Dijkstras, then discarded.
type UnifiedCSR struct {
	NodeCount uint32
	EdgeCount uint32
	Offsets   []uint32          // [NodeCount+1]
	Edges     []PrecomputedEdge // [EdgeCount]

	// Reverse mapping: dense node → tile origin (for writing landmarks back)
	NodeTileID  []uint32 // [NodeCount]
	NodeLocalID []uint32 // [NodeCount]
}

// Close releases all memory.
func (csr *UnifiedCSR) Close() {
	csr.Offsets = nil
	csr.Edges = nil
	csr.NodeTileID = nil
	csr.NodeLocalID = nil
	debug.FreeOSMemory()
}

// EdgeCostFunc computes edge cost factors from tag bits.
type EdgeCostFunc func(tags1, tags2 uint64) EdgeCostFactors

// BuildUnifiedCSR merges all tiles into a flat CSR for Dijkstra preprocessing.
// Border nodes are deduplicated via CoordKeyToInt64. Cross-tile edges arise
// automatically: the same physical node in 2 tiles gets the same denseID,
// so local edges from both tiles connect the graph.
func BuildUnifiedCSR(
	ctx context.Context,
	tileDir string,
	edgeCostFn EdgeCostFunc,
	progressFn func(step string, current, total int),
) (*UnifiedCSR, map[uint32][]uint32, error) {
	if progressFn == nil {
		progressFn = func(string, int, int) {}
	}
	t0 := time.Now()

	// Phase 1: Scan tiles, build node mapping with border dedup
	progressFn("scanning tiles", 0, 0)
	tileFiles, err := ScanTilesForLandmarks(tileDir)
	if err != nil {
		return nil, nil, fmt.Errorf("scan tiles: %w", err)
	}
	if len(tileFiles) == 0 {
		return nil, nil, fmt.Errorf("no tiles found in %s", tileDir)
	}

	borderDedup := make(map[int64]uint32, len(tileFiles)*1000) // CoordKey → denseID
	nodeMapping := make(map[uint32][]uint32, len(tileFiles))   // tileID → localID → denseID
	var nextDenseID uint32

	for ti, tf := range tileFiles {
		tilePath := TileFilePath(tileDir, tf.TileID)
		g, err := DeserializeMmap(tilePath)
		if err != nil {
			slog.Warn("skip tile for CSR build", "tile", tf.TileID, "error", err)
			continue
		}

		mapping := make([]uint32, g.NodeCount)
		for i := range g.NodeCount {
			if g.IsBorder != nil && i < uint32(len(g.IsBorder)) && g.IsBorder[i] {
				coordKey := CoordKeyToInt64(CoordKey{Lat: g.NodeLat[i], Lon: g.NodeLon[i]})
				if existing, ok := borderDedup[coordKey]; ok {
					mapping[i] = existing
					continue
				}
				borderDedup[coordKey] = nextDenseID
			}
			mapping[i] = nextDenseID
			nextDenseID++
		}
		nodeMapping[tf.TileID] = mapping
		g.closeMmap()

		if (ti+1)%100 == 0 {
			progressFn("node mapping", ti+1, len(tileFiles))
		}
	}

	dedupedCount := 0
	for _, tf := range tileFiles {
		dedupedCount += int(tf.NodeCount)
	}
	dedupedCount -= int(nextDenseID)

	slog.Info("unified CSR: node mapping complete",
		"totalNodes", nextDenseID,
		"dedupedBorderNodes", dedupedCount,
		"tiles", len(tileFiles),
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	// Free borderDedup — not needed after Phase 1
	borderDedup = nil

	csr := &UnifiedCSR{NodeCount: nextDenseID}

	// Phase 2, Pass 1: Count edges per dense node
	progressFn("counting edges", 0, len(tileFiles))
	edgeCounts := make([]uint32, nextDenseID)

	for ti, tf := range tileFiles {
		mapping, ok := nodeMapping[tf.TileID]
		if !ok {
			continue
		}
		tilePath := TileFilePath(tileDir, tf.TileID)
		g, err := DeserializeMmap(tilePath)
		if err != nil {
			slog.Warn("skip tile in edge count", "tile", tf.TileID, "error", err)
			continue
		}

		for i := range g.NodeCount {
			denseID := mapping[i]
			edgeCount := g.Offsets[i+1] - g.Offsets[i]
			edgeCounts[denseID] += edgeCount
		}
		g.closeMmap()

		if (ti+1)%100 == 0 {
			progressFn("counting edges", ti+1, len(tileFiles))
		}
	}

	// Build prefix sum → Offsets
	csr.Offsets = make([]uint32, nextDenseID+1)
	for i := range nextDenseID {
		csr.Offsets[i+1] = csr.Offsets[i] + edgeCounts[i]
	}
	csr.EdgeCount = csr.Offsets[nextDenseID]
	edgeCounts = nil // free

	slog.Info("unified CSR: edge count complete",
		"edges", csr.EdgeCount,
		"elapsed", time.Since(t0).Round(time.Millisecond),
	)

	// Phase 2, Pass 2: Write edges
	progressFn("writing edges", 0, len(tileFiles))
	csr.Edges = make([]PrecomputedEdge, csr.EdgeCount)
	csr.NodeTileID = make([]uint32, nextDenseID)
	csr.NodeLocalID = make([]uint32, nextDenseID)
	writePos := make([]uint32, nextDenseID)
	copy(writePos, csr.Offsets[:nextDenseID])

	for ti, tf := range tileFiles {
		mapping, ok := nodeMapping[tf.TileID]
		if !ok {
			continue
		}
		tilePath := TileFilePath(tileDir, tf.TileID)
		g, err := DeserializeMmap(tilePath)
		if err != nil {
			// MUST match Pass 1 — if this fails but Pass 1 succeeded, abort
			return nil, nil, fmt.Errorf("tile %d: pass 2 load failed (pass 1 succeeded): %w", tf.TileID, err)
		}

		for i := range g.NodeCount {
			denseID := mapping[i]
			// Set reverse mapping (last tile wins for border nodes — fine)
			csr.NodeTileID[denseID] = tf.TileID
			csr.NodeLocalID[denseID] = i

			for ei := g.Offsets[i]; ei < g.Offsets[i+1]; ei++ {
				target := g.Targets[ei]
				targetDense := mapping[target]

				ecf := edgeCostFn(g.EdgeTags1[ei], g.EdgeTags2[ei])
				cf := ecf.Flat
				if ecf.Up > 0 && ecf.Up < cf {
					cf = ecf.Up
				}
				if ecf.Down > 0 && ecf.Down < cf {
					cf = ecf.Down
				}

				pos := writePos[denseID]
				csr.Edges[pos] = PrecomputedEdge{
					Target:     targetDense,
					Dist:       g.EdgeDist[ei],
					CostFactor: cf,
				}
				writePos[denseID]++
			}
		}
		g.closeMmap()

		if (ti+1)%100 == 0 {
			progressFn("writing edges", ti+1, len(tileFiles))
		}
	}
	writePos = nil

	slog.Info("unified CSR: build complete",
		"nodes", csr.NodeCount, "edges", csr.EdgeCount,
		"elapsed", time.Since(t0).Round(time.Second),
	)

	_ = ctx // reserved for future cancellation support

	return csr, nodeMapping, nil
}
