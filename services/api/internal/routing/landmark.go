package routing

import (
	"math"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// LandmarkHeuristic caches destination distances for fast per-node lookups.
type LandmarkHeuristic struct {
	reader   GraphReader
	setID    LandmarkSetID
	dstDists [LandmarksPerSet]float32
	valid    bool
}

// NewLandmarkHeuristic creates a heuristic pre-loaded with destination distances.
func NewLandmarkHeuristic(reader GraphReader, dstNode uint64, setID LandmarkSetID) *LandmarkHeuristic {
	h := &LandmarkHeuristic{reader: reader, setID: setID}
	dists := reader.LandmarkDists(setID, dstNode)
	if dists != nil && len(dists) == LandmarksPerSet {
		copy(h.dstDists[:], dists)
		h.valid = true
	}
	return h
}

// LowerBound returns the ALT lower bound for the cost from nodeID to the destination.
func (h *LandmarkHeuristic) LowerBound(nodeID uint64) float32 {
	if !h.valid {
		return 0
	}
	nodeDists := h.reader.LandmarkDists(h.setID, nodeID)
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

// equirectDistKm computes the equirectangular approximation distance in km
// between two fixed-point coordinates (×1e7).
func equirectDistKm(lat1, lon1, lat2, lon2 int32) float64 {
	const degToRad = 1.745329252e-9 // pi / (180 * 1e7)
	const earthRKm = 6371.0
	dlat := float64(lat2-lat1) * degToRad
	dlon := float64(lon2-lon1) * degToRad
	midLat := float64(lat1+lat2) / 2.0 * degToRad
	cosLat := math.Cos(midLat)
	dx := dlon * cosLat
	dy := dlat
	return earthRKm * math.Sqrt(dx*dx+dy*dy)
}
