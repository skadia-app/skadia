package graph

import (
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"slices"
	"time"
)

// WriteLandmarkSectionsForSet writes one landmark distance set + metadata into each tile.
// Per-Set-Strategie: each set is written separately.
// When writing Set B, Set A's data is carried over from cold sections
// (Serialize overwrites the entire file).
func WriteLandmarkSectionsForSet(
	tileDir string,
	tiles []TileScanResult,
	meta *LandmarkMetaData,
	sectionID uint16,
	tileDistArrays map[uint32][]float32,
) error {
	isFirstSet := sectionID == SectionLandmarkDistA
	written := 0

	for _, tile := range tiles {
		dists, ok := tileDistArrays[tile.TileID]
		if !ok {
			continue
		}

		tilePath := TileFilePath(tileDir, tile.TileID)

		// Load existing tile via mmap
		g, err := DeserializeMmap(tilePath)
		if err != nil {
			slog.Warn("skip tile for landmark write", "tile", tile.TileID, "error", err)
			continue
		}

		if g.pendingLandmarkSections == nil {
			g.pendingLandmarkSections = make(map[uint16][]float32)
		}

		// Add THIS set's distance array
		g.pendingLandmarkSections[sectionID] = dists

		// Carry over PREVIOUSLY written landmark sets from cold storage
		for prevSetID := SectionLandmarkDistA; prevSetID < sectionID; prevSetID++ {
			if _, alreadyPending := g.pendingLandmarkSections[prevSetID]; alreadyPending {
				continue
			}
			entry, hasCold := g.coldSections[prevSetID]
			if !hasCold {
				continue
			}
			prevDists, readErr := ReadColdFloat32Section(g.backingFile, entry, 0,
				uint32(g.NodeCount)*LandmarksPerSet)
			if readErr != nil {
				slog.Warn("could not carry over previous landmark set",
					"tile", tile.TileID, "section", prevSetID, "error", readErr)
				continue
			}
			g.pendingLandmarkSections[prevSetID] = prevDists
		}

		// Write LandmarkMeta on first set (or if missing)
		if isFirstSet || g.LandmarkMeta == nil {
			g.LandmarkMeta = meta
		}

		// Re-serialize
		if err := Serialize(g, tilePath); err != nil {
			slog.Warn("landmark write failed", "tile", tile.TileID, "error", err)
			g.closeMmap()
			continue
		}

		// Release mmap handle
		g.closeMmap()

		written++
		if written%100 == 0 {
			slog.Info("landmark sections progress", "section", sectionID,
				"written", written, "total", len(tiles))
		}
	}

	slog.Info("landmark sections written", "section", sectionID,
		"written", written, "total", len(tiles))
	return nil
}

// TileScanResult holds per-tile node coordinates from a landmark scan pass.
type TileScanResult struct {
	TileID    uint32
	NodeCount uint32
	NodeLats  []int32
	NodeLons  []int32
	NodeEles  []int32
}

// ScanTileForLandmarks loads a tile via mmap, copies node coordinates, and releases the mmap.
func ScanTileForLandmarks(tilePath string) (*TileScanResult, error) {
	g, err := DeserializeMmap(tilePath)
	if err != nil {
		return nil, err
	}
	// CRITICAL: Copy slices — mmap-backed zero-copy slices become invalid after mmap release.
	lats := make([]int32, g.NodeCount)
	copy(lats, g.NodeLat)
	lons := make([]int32, g.NodeCount)
	copy(lons, g.NodeLon)
	eles := make([]int32, g.NodeCount)
	copy(eles, g.NodeEle)

	// Release mmap resources.
	g.closeMmap()

	return &TileScanResult{
		TileID:    g.TileID,
		NodeCount: g.NodeCount,
		NodeLats:  lats,
		NodeLons:  lons,
		NodeEles:  eles,
	}, nil
}

// ScanTilesForLandmarks scans all tile_*.bin files in tileDir and returns per-tile node data.
func ScanTilesForLandmarks(tileDir string) ([]TileScanResult, error) {
	pattern := filepath.Join(tileDir, "tile_*.bin")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob tiles: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no tile files found in %s", tileDir)
	}

	var results []TileScanResult
	for _, path := range matches {
		result, err := ScanTileForLandmarks(path)
		if err != nil {
			slog.Warn("skip tile for landmark scan", "path", path, "error", err)
			continue
		}
		results = append(results, *result)
	}

	slog.Info("landmark scan complete", "tiles", len(results))
	return results, nil
}

// DenseIndex maps global node IDs to a contiguous dense integer space for landmark distance arrays.
type DenseIndex struct {
	TileOffset map[uint32]uint32
	TotalNodes uint32
}

// BuildDenseIndex builds a dense index from a list of tile scan results.
func BuildDenseIndex(tiles []TileScanResult) *DenseIndex {
	idx := &DenseIndex{TileOffset: make(map[uint32]uint32, len(tiles))}
	var offset uint32
	for _, t := range tiles {
		idx.TileOffset[t.TileID] = offset
		offset += t.NodeCount
	}
	idx.TotalNodes = offset
	return idx
}

// ToDense maps a global node ID to its dense array index. Returns math.MaxUint32 if not found.
func (idx *DenseIndex) ToDense(globalNodeID uint64) uint32 {
	tileID, localID := SplitGlobalNodeID(globalNodeID)
	offset, ok := idx.TileOffset[tileID]
	if !ok {
		return math.MaxUint32
	}
	dense := offset + localID
	if dense >= idx.TotalNodes {
		return math.MaxUint32
	}
	return dense
}

// LandmarkCandidate is a node selected as a potential ALT landmark.
type LandmarkCandidate struct {
	GlobalNodeID uint64
	Lat, Lon     int32
	Ele          int32
}

// SelectLandmarks selects up to 16 landmarks from the given tiles using a combination of:
//   - Lowest-elevation nodes (4)
//   - Highest-elevation nodes (4)
//   - BBox corner-nearest nodes (4)
//   - Farthest-by-Dijkstra nodes (remaining slots, optional)
//   - Elevation-spread fill (fallback for incomplete sets)
//
// farthestDijkstra may be nil to skip graph-distance-based selection.
func SelectLandmarks(
	tiles []TileScanResult,
	farthestDijkstra func(src uint64) (LandmarkCandidate, error),
) (*LandmarkMetaData, error) {
	if len(tiles) == 0 {
		return nil, fmt.Errorf("no tiles to select landmarks from")
	}

	type nodeRef struct {
		globalID uint64
		lat, lon int32
		ele      int32
	}

	var minLat, maxLat, minLon, maxLon int32
	minLat, minLon = math.MaxInt32, math.MaxInt32
	maxLat, maxLon = math.MinInt32, math.MinInt32

	var highest [4]nodeRef
	var lowest [4]nodeRef
	highCount, lowCount := 0, 0
	var totalNodes int

	for _, t := range tiles {
		totalNodes += int(t.NodeCount)
		for i := range t.NodeCount {
			lat, lon, ele := t.NodeLats[i], t.NodeLons[i], t.NodeEles[i]
			gid := MakeGlobalNodeID(t.TileID, i)

			if lat < minLat {
				minLat = lat
			}
			if lat > maxLat {
				maxLat = lat
			}
			if lon < minLon {
				minLon = lon
			}
			if lon > maxLon {
				maxLon = lon
			}

			nr := nodeRef{globalID: gid, lat: lat, lon: lon, ele: ele}

			// Track 4 highest elevations (sorted ascending by ele; [0] is minimum of the top-4).
			if highCount < 4 {
				highest[highCount] = nr
				highCount++
				if highCount == 4 {
					slices.SortFunc(highest[:], func(a, b nodeRef) int { return int(a.ele - b.ele) })
				}
			} else if ele > highest[0].ele {
				highest[0] = nr
				slices.SortFunc(highest[:], func(a, b nodeRef) int { return int(a.ele - b.ele) })
			}

			// Track 4 lowest elevations (sorted descending by ele; [0] is maximum of the bottom-4).
			if lowCount < 4 {
				lowest[lowCount] = nr
				lowCount++
				if lowCount == 4 {
					slices.SortFunc(lowest[:], func(a, b nodeRef) int { return int(b.ele - a.ele) })
				}
			} else if ele < lowest[0].ele {
				lowest[0] = nr
				slices.SortFunc(lowest[:], func(a, b nodeRef) int { return int(b.ele - a.ele) })
			}
		}
	}

	slog.Info("landmark selection: scanned nodes", "total", totalNodes,
		"bbox", fmt.Sprintf("lat[%d,%d] lon[%d,%d]", minLat, maxLat, minLon, maxLon))

	var landmarks [16]LandmarkCandidate
	used := make(map[uint64]bool)
	pickIdx := 0

	pick := func(c LandmarkCandidate) bool {
		if pickIdx >= 16 || used[c.GlobalNodeID] {
			return false
		}
		landmarks[pickIdx] = c
		used[c.GlobalNodeID] = true
		pickIdx++
		return true
	}

	toCandidate := func(nr nodeRef) LandmarkCandidate {
		return LandmarkCandidate{GlobalNodeID: nr.globalID, Lat: nr.lat, Lon: nr.lon, Ele: nr.ele}
	}

	// Pick 4 lowest-elevation nodes.
	for i := range min(4, lowCount) {
		pick(toCandidate(lowest[i]))
	}

	// Pick 4 highest-elevation nodes (highest first: iterate descending).
	for i := min(4, highCount) - 1; i >= 0; i-- {
		pick(toCandidate(highest[i]))
	}

	// BBox corners with cosLat correction for longitude distances.
	midLatRad := float64(minLat+maxLat) / 2.0 * 1.745329252e-9 // ×1e-7 × π/180
	cosLat := math.Cos(midLatRad)

	corners := [][2]int32{
		{minLat, minLon}, {minLat, maxLon},
		{maxLat, minLon}, {maxLat, maxLon},
	}
	for _, corner := range corners {
		best := findNearestCosLat(tiles, corner[0], corner[1], cosLat, used)
		if best.GlobalNodeID != 0 {
			pick(best)
		}
	}

	// Farthest-point iteration via Dijkstra (optional).
	if farthestDijkstra != nil && pickIdx > 0 {
		src := landmarks[0].GlobalNodeID
		for pickIdx < 16 {
			candidate, err := farthestDijkstra(src)
			if err != nil {
				slog.Warn("farthest dijkstra failed", "error", err, "idx", pickIdx)
				break
			}
			if candidate.GlobalNodeID == 0 || used[candidate.GlobalNodeID] {
				break
			}
			pick(candidate)
			src = candidate.GlobalNodeID
		}
	}

	// Elevation-spread fallback if still under 16.
	if pickIdx < 16 {
		slog.Warn("landmark selection incomplete", "count", pickIdx, "target", 16)
		fillLandmarksFromElevSpread(tiles, &landmarks, &pickIdx, used)
	}

	meta := &LandmarkMetaData{
		UphillThreshold:   7.0,
		DownhillThreshold: 7.0,
		LandmarkCount:     pickIdx,
		PreprocessingTime: time.Now(),
	}
	for i := range pickIdx {
		meta.NodeIDs[i] = landmarks[i].GlobalNodeID
		meta.Lats[i] = landmarks[i].Lat
		meta.Lons[i] = landmarks[i].Lon
	}

	slog.Info("landmark selection complete", "total", pickIdx)
	return meta, nil
}

// findNearestCosLat finds the closest unused node to (targetLat, targetLon)
// using squared distance with cosLat correction on longitude.
func findNearestCosLat(tiles []TileScanResult, targetLat, targetLon int32, cosLat float64, used map[uint64]bool) LandmarkCandidate {
	var best LandmarkCandidate
	bestDist := math.MaxFloat64
	cosLat2 := cosLat * cosLat

	for _, t := range tiles {
		for i := range t.NodeCount {
			gid := MakeGlobalNodeID(t.TileID, i)
			if used[gid] {
				continue
			}
			dlat := float64(t.NodeLats[i] - targetLat)
			dlon := float64(t.NodeLons[i] - targetLon)
			d := dlat*dlat + dlon*dlon*cosLat2
			if d < bestDist {
				bestDist = d
				best = LandmarkCandidate{
					GlobalNodeID: gid,
					Lat:          t.NodeLats[i],
					Lon:          t.NodeLons[i],
					Ele:          t.NodeEles[i],
				}
			}
		}
	}
	return best
}

// fillLandmarksFromElevSpread fills remaining landmark slots by sampling nodes
// at every 1000th position and picking evenly spaced elevations.
func fillLandmarksFromElevSpread(tiles []TileScanResult, landmarks *[16]LandmarkCandidate, pickIdx *int, used map[uint64]bool) {
	var sample []LandmarkCandidate
	nth := 0
	for _, t := range tiles {
		for i := range t.NodeCount {
			nth++
			if nth%1000 != 0 {
				continue
			}
			gid := MakeGlobalNodeID(t.TileID, i)
			if used[gid] {
				continue
			}
			sample = append(sample, LandmarkCandidate{
				GlobalNodeID: gid,
				Lat:          t.NodeLats[i],
				Lon:          t.NodeLons[i],
				Ele:          t.NodeEles[i],
			})
		}
	}
	if len(sample) == 0 {
		return
	}
	slices.SortFunc(sample, func(a, b LandmarkCandidate) int { return int(a.Ele - b.Ele) })
	step := max(1, len(sample)/(16-*pickIdx+1))
	for i := step; *pickIdx < 16 && i < len(sample); i += step {
		if !used[sample[i].GlobalNodeID] {
			landmarks[*pickIdx] = sample[i]
			used[sample[i].GlobalNodeID] = true
			*pickIdx++
		}
	}
}
