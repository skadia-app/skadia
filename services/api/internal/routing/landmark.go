package routing

import (
	"math"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// landmarkPageSize is the number of nodes fetched per ReadAt call.
// 64 nodes × 16 landmarks × 4 bytes = 4096 bytes = exactly one OS page.
// Reduces ReadAt syscalls from ~6.8 M to ~26 K for 1.7 M-expansion routes.
const landmarkPageSize = 64

// LandmarkHeuristic caches destination distances for fast per-node lookups.
// Pre-allocated buffers (nodeBuf, byteBuf) eliminate heap allocations in the
// A* hot path — LandmarkHeuristic is created once per route, not shared.
//
// Page cache: LowerBound batch-reads 64 nodes per ReadAt call so that 63 of
// every 64 accesses are pure memory copies (~50 ns) instead of pread64 syscalls
// (~4.4 µs).  No interface change is needed for the cache logic itself; only
// LandmarkDistsPageInto on GraphReader adds the bulk-read capability.
type LandmarkHeuristic struct {
	reader   GraphReader
	setID    LandmarkSetID
	dstDists [LandmarksPerSet]float32
	valid    bool
	// Fallback single-node buffers (used when page load fails).
	nodeBuf [LandmarksPerSet]float32
	byteBuf [LandmarksPerSet * 4]byte

	// Page cache: one OS-page worth of landmark distances loaded at a time.
	pageBuf     [landmarkPageSize * LandmarksPerSet]float32
	pageByteBuf [landmarkPageSize * LandmarksPerSet * 4]byte
	pageTileID  uint32
	pageNum     int32 // -1 = invalid / not yet loaded
}

// NewLandmarkHeuristic creates a heuristic pre-loaded with destination distances.
func NewLandmarkHeuristic(reader GraphReader, dstNode uint64, setID LandmarkSetID) *LandmarkHeuristic {
	h := &LandmarkHeuristic{
		reader:  reader,
		setID:   setID,
		pageNum: -1, // no page cached yet
	}
	dists := reader.LandmarkDists(setID, dstNode)
	if dists != nil && len(dists) == LandmarksPerSet {
		copy(h.dstDists[:], dists)
		h.valid = true
	}
	return h
}

// LowerBound returns the ALT lower bound for the cost from nodeID to the destination.
// Uses a page-level cache: the first access to any 64-node page triggers one
// ReadAt; the remaining 63 nodes on the same page are served from h.pageBuf.
func (h *LandmarkHeuristic) LowerBound(nodeID uint64) float32 {
	if !h.valid {
		return 0
	}

	tileID, localID := SplitGlobalNodeID(nodeID)
	pageNum := int32(localID / landmarkPageSize)

	if h.pageNum != pageNum || h.pageTileID != tileID {
		// Cache miss: read a full page (up to 64 nodes) in one I/O call.
		startNode := uint32(pageNum) * landmarkPageSize
		page := h.reader.LandmarkDistsPageInto(
			h.setID, tileID, startNode, landmarkPageSize,
			h.pageBuf[:], h.pageByteBuf[:],
		)
		if page == nil {
			// Fallback: single-node read (e.g. cold section unavailable).
			nodeDists := h.reader.LandmarkDistsInto(h.setID, nodeID, h.nodeBuf[:], h.byteBuf[:])
			return h.lowerBoundFromDists(nodeDists)
		}
		h.pageNum = pageNum
		h.pageTileID = tileID
	}

	// Page cache hit: extract the LandmarksPerSet floats for this node.
	// Stride is LandmarksPerSet (16), not landmarkPageSize (64).
	off := (localID % landmarkPageSize) * LandmarksPerSet
	return h.lowerBoundFromDists(h.pageBuf[off : off+LandmarksPerSet])
}

// lowerBoundFromDists computes max |nodeDists[k] - dstDists[k]| over all landmarks.
func (h *LandmarkHeuristic) lowerBoundFromDists(nodeDists []float32) float32 {
	if nodeDists == nil {
		return 0
	}
	var best float32
	for k := range LandmarksPerSet {
		if nodeDists[k] >= math.MaxFloat32 || h.dstDists[k] >= math.MaxFloat32 {
			continue
		}
		diff := nodeDists[k] - h.dstDists[k]
		if diff < 0 {
			diff = -diff
		}
		if diff > best {
			best = diff
		}
	}
	return best
}

// SelectLandmarkSet returns the appropriate landmark set for the given cost config.
// The set is chosen based on whether uphill/downhill costs exceed the thresholds
// used during landmark preprocessing.
func SelectLandmarkSet(cfg *CostConfig, meta *graph.LandmarkMetaData) LandmarkSetID {
	if meta == nil {
		return LandmarkSetD
	}
	hasUphill := cfg.UphillCost >= meta.UphillThreshold
	hasDownhill := cfg.DownhillCost >= meta.DownhillThreshold
	switch {
	case hasUphill && hasDownhill:
		return LandmarkSetA
	case hasUphill:
		return LandmarkSetB
	case hasDownhill:
		return LandmarkSetC
	default:
		return LandmarkSetD
	}
}

// EstimateAvgCostPerKm estimates the average routing cost per km of air distance
// by sampling landmark distances from the start node and computing a median ratio.
func EstimateAvgCostPerKm(reader GraphReader, startNode uint64, setID LandmarkSetID) float32 {
	dists := reader.LandmarkDists(setID, startNode)
	if dists == nil {
		return 0
	}
	meta := reader.LandmarkMeta()
	if meta == nil {
		return 0
	}

	startLat, startLon, _ := reader.NodeCoord(startNode)

	var ratios []float32
	for k := range LandmarksPerSet {
		if dists[k] <= 0 || dists[k] >= math.MaxFloat32 {
			continue
		}
		airDistKm := equirectDistKm(startLat, startLon, meta.Lats[k], meta.Lons[k])
		if airDistKm < 1.0 {
			continue
		}
		ratios = append(ratios, dists[k]/float32(airDistKm))
	}

	if len(ratios) < 4 {
		return 0
	}
	slices.SortFunc(ratios, func(a, b float32) int {
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	})
	return ratios[len(ratios)/2]
}

// ComputeAvgCostfactor estimates the average flat costfactor for a given EdgeCost closure.
// Uses a representative distribution of highway classes weighted by expected trail mix.
func ComputeAvgCostfactor(edgeCost func(tags1, tags2 uint64) EdgeCostFactors) float32 {
	type hwWeight struct {
		hwBits uint64
		weight float32
	}
	distribution := []hwWeight{
		{3, 0.05},  // primary
		{4, 0.10},  // secondary
		{5, 0.15},  // tertiary
		{6, 0.30},  // residential
		{8, 0.20},  // track
		{9, 0.15},  // path
		{11, 0.05}, // cycleway
	}

	var sum float32
	for _, hw := range distribution {
		ecf := edgeCost(hw.hwBits, 0)
		if ecf.Flat < costBlocked {
			sum += ecf.Flat * hw.weight
		}
	}
	return sum
}

// ScaledAvgCostPerKm adjusts the landmark-based cost/km to the actual profile.
// Scales landmarkAvg by the ratio of profileCf to landmarkCf.
func ScaledAvgCostPerKm(landmarkAvg, profileCf, landmarkCf float32) float32 {
	if landmarkCf <= 0 {
		return landmarkAvg
	}
	return landmarkAvg * (profileCf / landmarkCf)
}

// equirectDistKm computes approximate distance in km between two int32 coords (×1e7).
// Uses standard math.Cos for build-time precision.
// See also: equirectDist() in distance.go (float32 meters, Bhaskara approx for hot path).
func equirectDistKm(lat1, lon1, lat2, lon2 int32) float64 {
	dlat := float64(lat2-lat1) * degToRadFP
	dlon := float64(lon2-lon1) * degToRadFP
	midLat := float64(lat1+lat2) / 2.0 * degToRadFP
	cosLat := math.Cos(midLat)
	dx := dlon * cosLat
	dy := dlat
	return earthRKm * math.Sqrt(dx*dx+dy*dy)
}
