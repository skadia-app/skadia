package edgetags

import (
	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

type CoordRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type AllRanges struct {
	Forest    []CoordRange
	Protected []CoordRange
	Sac       map[string][]CoordRange
	Mtb       map[string][]CoordRange
	Vf        map[string][]CoordRange
	VfUnknown []CoordRange
	Surface   map[string][]CoordRange
	Highway   map[string][]CoordRange
}

const (
	ForestHighlightMin    uint8 = 60
	ProtectedHighlightMin uint8 = 1
)

var SacLevels = [6]string{"T1", "T2", "T3", "T4", "T5", "T6"}
var MtbLevels = [6]string{"S0", "S1", "S2", "S3", "S4", "S5"}
var VfLevels = [6]string{"K1", "K2", "K3", "K4", "K5", "K6"}

func ContiguousRangesFromMatch(matches []bool, edgeCoordStart []int, lastCoordIdx int) []CoordRange {
	nEdges := len(matches)
	if nEdges == 0 || len(edgeCoordStart) != nEdges {
		return nil
	}

	var out []CoordRange
	var current *CoordRange
	for i := 0; i < nEdges; i++ {
		if !matches[i] {
			if current != nil {
				out = append(out, *current)
				current = nil
			}
			continue
		}

		edgeFrom := edgeCoordStart[i]
		edgeTo := lastCoordIdx + 1
		if i+1 < nEdges {
			edgeTo = edgeCoordStart[i+1]
		}
		if current != nil && current.To == edgeFrom {
			current.To = edgeTo
			continue
		}
		if current != nil {
			out = append(out, *current)
		}
		current = &CoordRange{From: edgeFrom, To: edgeTo}
	}

	if current != nil {
		out = append(out, *current)
	}
	return out
}

func LastRealCoordIdx(coordsLen, lastEdgeCoordEnd int) int {
	if lastEdgeCoordEnd > 0 {
		return lastEdgeCoordEnd - 1
	}
	return coordsLen - 1
}

func ComputeAllRanges(edgeCoordStart []int, edgeTags1 []uint64, edgeTags2 []uint64, lastCoordIdx int) AllRanges {
	n := len(edgeTags1)
	if n == 0 || len(edgeTags2) != n || len(edgeCoordStart) != n {
		return AllRanges{}
	}

	forestM := make([]bool, n)
	protectedM := make([]bool, n)
	vfUnknownM := make([]bool, n)
	var sacM, mtbM, vfM [6][]bool
	for i := 0; i < 6; i++ {
		sacM[i] = make([]bool, n)
		mtbM[i] = make([]bool, n)
		vfM[i] = make([]bool, n)
	}

	for i := 0; i < n; i++ {
		t1 := edgeTags1[i]
		t2 := edgeTags2[i]

		if graph.DecodeForestPct(t2) >= ForestHighlightMin {
			forestM[i] = true
		}
		if graph.DecodeProtectedArea(t2) >= ProtectedHighlightMin {
			protectedM[i] = true
		}

		sacRaw := uint8((t1 >> 14) & 0x07)
		if sacRaw >= 1 && sacRaw <= 6 {
			sacM[sacRaw-1][i] = true
		}

		mtbRaw := uint8((t1 >> 17) & 0x07)
		if mtbRaw >= 1 && mtbRaw <= 6 {
			mtbM[mtbRaw-1][i] = true
		}

		vfRaw := graph.DecodeViaFerrataScale(t2)
		if vfRaw >= 1 && vfRaw <= 6 {
			vfM[vfRaw-1][i] = true
		}
		if vfRaw == 0 && graph.DecodeHighway(t1) == graph.HighwayViaFerrata {
			vfUnknownM[i] = true
		}
	}

	result := AllRanges{
		Forest:    ContiguousRangesFromMatch(forestM, edgeCoordStart, lastCoordIdx),
		Protected: ContiguousRangesFromMatch(protectedM, edgeCoordStart, lastCoordIdx),
		VfUnknown: ContiguousRangesFromMatch(vfUnknownM, edgeCoordStart, lastCoordIdx),
	}

	for i, level := range SacLevels {
		if ranges := ContiguousRangesFromMatch(sacM[i], edgeCoordStart, lastCoordIdx); len(ranges) > 0 {
			if result.Sac == nil {
				result.Sac = make(map[string][]CoordRange, 6)
			}
			result.Sac[level] = ranges
		}
	}
	for i, level := range MtbLevels {
		if ranges := ContiguousRangesFromMatch(mtbM[i], edgeCoordStart, lastCoordIdx); len(ranges) > 0 {
			if result.Mtb == nil {
				result.Mtb = make(map[string][]CoordRange, 6)
			}
			result.Mtb[level] = ranges
		}
	}
	for i, level := range VfLevels {
		if ranges := ContiguousRangesFromMatch(vfM[i], edgeCoordStart, lastCoordIdx); len(ranges) > 0 {
			if result.Vf == nil {
				result.Vf = make(map[string][]CoordRange, 6)
			}
			result.Vf[level] = ranges
		}
	}

	if len(result.Forest) == 0 {
		result.Forest = nil
	}
	if len(result.Protected) == 0 {
		result.Protected = nil
	}
	if len(result.VfUnknown) == 0 {
		result.VfUnknown = nil
	}
	if len(result.Sac) == 0 {
		result.Sac = nil
	}
	if len(result.Mtb) == 0 {
		result.Mtb = nil
	}
	if len(result.Vf) == 0 {
		result.Vf = nil
	}

	return result
}

type LatLng struct {
	Lat float64
	Lng float64
}

// ComputeTypeRangesFromLabels builds a map from each unique label to the
// contiguous coord-index ranges where that label appears.
// labels[i] is the classification of the i-th resampled point;
// the resulting CoordRange values use the same index space (0-based point index).
// Empty labels are skipped. Returns nil when labels is empty.
func ComputeTypeRangesFromLabels(labels []string) map[string][]CoordRange {
	n := len(labels)
	if n == 0 {
		return nil
	}

	// Collect unique types so we can build per-type boolean masks.
	seen := make(map[string]struct{}, 8)
	for _, l := range labels {
		if l != "" {
			seen[l] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil
	}

	result := make(map[string][]CoordRange, len(seen))
	for typ := range seen {
		// Build a boolean mask for this type.
		mask := make([]bool, n)
		for i, l := range labels {
			mask[i] = l == typ
		}

		// edgeCoordStart = [0, 1, 2, ..., n-1] — one "edge" per resampled point.
		edgeCoordStart := make([]int, n)
		for i := range n {
			edgeCoordStart[i] = i
		}

		ranges := ContiguousRangesFromMatch(mask, edgeCoordStart, n-1)
		if len(ranges) > 0 {
			result[typ] = ranges
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func AggregateForestProtected(coords []LatLng, edgeCoordStart []int, edgeTags2 []uint64, lastRealCoordIdx int) (forestPct, protectedPct float64) {
	n := len(edgeTags2)
	if n == 0 || len(edgeCoordStart) != n || len(coords) < 2 {
		return 0, 0
	}

	var totalDist float64
	var forestWeighted float64
	var protectedWeighted float64

	for i := 0; i < n; i++ {
		startIdx := edgeCoordStart[i]
		endIdx := lastRealCoordIdx
		if i+1 < n {
			endIdx = edgeCoordStart[i+1]
		}
		if endIdx > lastRealCoordIdx+1 {
			endIdx = lastRealCoordIdx + 1
		}
		if startIdx < 0 || endIdx >= len(coords) || startIdx >= endIdx {
			continue
		}

		var edgeDist float64
		for j := startIdx + 1; j <= endIdx; j++ {
			edgeDist += geo.DistanceHaversine(
				coords[j-1].Lat, coords[j-1].Lng,
				coords[j].Lat, coords[j].Lng,
			)
		}
		totalDist += edgeDist

		forestRaw := int(graph.DecodeForestPct(edgeTags2[i]))
		if forestRaw > 100 {
			forestRaw = 100
		}
		forestWeighted += (float64(forestRaw) / 100.0) * edgeDist
		if graph.DecodeProtectedArea(edgeTags2[i]) > 0 {
			protectedWeighted += edgeDist
		}
	}

	if totalDist == 0 {
		return 0, 0
	}
	return forestWeighted / totalDist, protectedWeighted / totalDist
}

func CoalesceConsecutive(edgeIdxs []uint32, tags1, tags2 []uint64) (edgeCoordStart []int, t1Out, t2Out []uint64) {
	n := len(edgeIdxs)
	if n == 0 || len(tags1) != n || len(tags2) != n {
		return nil, nil, nil
	}

	edgeCoordStart = make([]int, 0, n)
	t1Out = make([]uint64, 0, n)
	t2Out = make([]uint64, 0, n)
	var last uint32
	for i, edgeIdx := range edgeIdxs {
		if i > 0 && edgeIdx == last {
			continue
		}
		edgeCoordStart = append(edgeCoordStart, i)
		t1Out = append(t1Out, tags1[i])
		t2Out = append(t2Out, tags2[i])
		last = edgeIdx
	}
	return edgeCoordStart, t1Out, t2Out
}
