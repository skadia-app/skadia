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
