package handler

import "github.com/skadia-app/skadia/services/api/internal/routing/graph"

// CoordRange represents a contiguous index range into a route's coordinate list.
// Used by the frontend to render map highlights for sub-route sections.
type CoordRange struct {
	From int `json:"from"` // inclusive, index in RouteSuggestion.geojson coordinates
	To   int `json:"to"`   // exclusive
}

// Edge-level match-thresholds for per-edge classification into category ranges.
// Values align with the pipeline's forest-enrichment (osm_ways.forest_pct >= 60
// sets landuse=forest; see osm/enrichment.go:609) and with the 2-bit
// protected_area encoding (any non-zero level counts).
const (
	forestHighlightMin    uint8 = 60
	protectedHighlightMin uint8 = 1
)

// contiguousRangesFromMatch collapses a per-edge boolean match array into
// contiguous CoordRanges over the route's coordinate list.
//
// matches[i] corresponds to the edge at edgeCoordStart[i]. The last edge's
// range extends to lastCoordIdx+1 (destination coord, exclusive upper bound).
// Non-matching edges break contiguous ranges.
func contiguousRangesFromMatch(
	matches []bool,
	edgeCoordStart []int,
	lastCoordIdx int,
) []CoordRange {
	var out []CoordRange
	var current *CoordRange
	nEdges := len(matches)
	for i := 0; i < nEdges; i++ {
		if !matches[i] {
			if current != nil {
				out = append(out, *current)
				current = nil
			}
			continue
		}
		edgeFrom := edgeCoordStart[i]
		var edgeTo int
		if i+1 < nEdges {
			edgeTo = edgeCoordStart[i+1]
		} else {
			edgeTo = lastCoordIdx + 1
		}
		if current != nil && current.To == edgeFrom {
			current.To = edgeTo
		} else {
			if current != nil {
				out = append(out, *current)
			}
			current = &CoordRange{From: edgeFrom, To: edgeTo}
		}
	}
	if current != nil {
		out = append(out, *current)
	}
	return out
}

// computeForestRanges builds contiguous CoordRanges for edges whose forest pct
// (decoded from Tags2) is ≥ forestHighlightMin. Pure in-memory — no DB / reader.
func computeForestRanges(
	edgeCoordStart []int,
	edgeTags2 []uint64,
	lastCoordIdx int,
) []CoordRange {
	matches := make([]bool, len(edgeTags2))
	for i, t2 := range edgeTags2 {
		matches[i] = graph.DecodeForestPct(t2) >= forestHighlightMin
	}
	return contiguousRangesFromMatch(matches, edgeCoordStart, lastCoordIdx)
}

// computeProtectedRanges builds contiguous CoordRanges for edges whose
// protected_area level (decoded from Tags2 bits 17-18) is ≥ protectedHighlightMin.
// Pure in-memory — no DB / reader.
func computeProtectedRanges(
	edgeCoordStart []int,
	edgeTags2 []uint64,
	lastCoordIdx int,
) []CoordRange {
	matches := make([]bool, len(edgeTags2))
	for i, t2 := range edgeTags2 {
		matches[i] = graph.DecodeProtectedArea(t2) >= protectedHighlightMin
	}
	return contiguousRangesFromMatch(matches, edgeCoordStart, lastCoordIdx)
}

// computeScaleRanges builds CoordRanges for edges matching a specific SAC or
// MTB scale level. kind is "sac" or "mtb"; level is "T1".."T6" or "S0".."S5".
//
// Bit layout (both from Tags1, per EncodeOSMTags in graph/tags.go):
//   - SAC scale: bits 14-16 (shift 14), mapped via graph.SacScaleShort (direct: raw=3 → "T3")
//   - MTB scale: bits 17-19 (shift 17), mapped via graph.MtbScaleShort (offset: raw=3 → "S2", NOT "S3")
//
// Unknown kind or level returns nil without panicking.
func computeScaleRanges(
	edgeCoordStart []int,
	edgeTags1 []uint64,
	lastCoordIdx int,
	kind string,
	level string,
) []CoordRange {
	matches := make([]bool, len(edgeTags1))
	switch kind {
	case "sac":
		for i, t1 := range edgeTags1 {
			raw := uint8((t1 >> 14) & 0x07)
			if graph.SacScaleShort(raw) == level {
				matches[i] = true
			}
		}
	case "mtb":
		for i, t1 := range edgeTags1 {
			raw := uint8((t1 >> 17) & 0x07)
			if graph.MtbScaleShort(raw) == level {
				matches[i] = true
			}
		}
	default:
		return nil
	}
	return contiguousRangesFromMatch(matches, edgeCoordStart, lastCoordIdx)
}
