package handler

import "github.com/skadia-app/skadia/services/api/internal/routing/graph"

// AllRanges holds all per-category CoordRange slices computed in a single pass
// over the edge-tag arrays. Populated by computeAllRanges.
//
// Nil maps/slices mean no edges matched that category (same semantics as empty).
type AllRanges struct {
	Forest    []CoordRange
	Protected []CoordRange
	// Sac, Mtb, Vf are keyed by level string (e.g. "T1".."T6", "S0".."S5", "K1".."K6").
	// nil means no edges matched any level.
	Sac       map[string][]CoordRange
	Mtb       map[string][]CoordRange
	Vf        map[string][]CoordRange
	VfUnknown []CoordRange
}

// sacLevels and mtbLevels are the canonical level keys; order is stable.
var sacLevels = [6]string{"T1", "T2", "T3", "T4", "T5", "T6"}
var mtbLevels = [6]string{"S0", "S1", "S2", "S3", "S4", "S5"}
var vfLevels = [6]string{"K1", "K2", "K3", "K4", "K5", "K6"}

// computeAllRanges scans edgeTags1 / edgeTags2 exactly once and produces all
// Range categories in one pass instead of 15 separate scan loops.
//
// Returns an empty AllRanges (all nil) if the three slice lengths are not equal
// or if n == 0.  Callers MUST pass matching-length slices — mismatched lengths
// indicate a programmer error in the route-build pipeline and are guarded here.
func computeAllRanges(
	edgeCoordStart []int,
	edgeTags1 []uint64,
	edgeTags2 []uint64,
	lastCoordIdx int,
) AllRanges {
	n := len(edgeTags1)
	if n == 0 || len(edgeTags2) != n || len(edgeCoordStart) != n {
		return AllRanges{}
	}

	// Pre-allocate per-edge match arrays for every category.
	forestM := make([]bool, n)
	protectedM := make([]bool, n)
	vfUnknownM := make([]bool, n)

	// Six bool-arrays each for SAC / MTB / VF.
	var sacM, mtbM, vfM [6][]bool
	for k := range sacM {
		sacM[k] = make([]bool, n)
		mtbM[k] = make([]bool, n)
		vfM[k] = make([]bool, n)
	}

	// Single pass over all edges.
	for i := 0; i < n; i++ {
		t1 := edgeTags1[i]
		t2 := edgeTags2[i]

		// Forest / Protected (Tags2).
		if graph.DecodeForestPct(t2) >= forestHighlightMin {
			forestM[i] = true
		}
		if graph.DecodeProtectedArea(t2) >= protectedHighlightMin {
			protectedM[i] = true
		}

		// SAC scale (Tags1 bits 14-16).
		sacRaw := uint8((t1 >> 14) & 0x07)
		if sacRaw >= 1 && sacRaw <= 6 {
			sacM[sacRaw-1][i] = true // T1=idx0, T6=idx5
		}

		// MTB scale (Tags1 bits 17-19).
		mtbRaw := uint8((t1 >> 17) & 0x07)
		if mtbRaw >= 1 && mtbRaw <= 6 {
			mtbM[mtbRaw-1][i] = true // raw=1→S0(idx0), raw=6→S5(idx5)
		}

		// VF scale (Tags2 bits 33-35).
		vfRaw := graph.DecodeViaFerrataScale(t2)
		hw := graph.DecodeHighway(t1)
		if vfRaw >= 1 && vfRaw <= 6 {
			vfM[vfRaw-1][i] = true // K1=idx0, K6=idx5
		}
		// VF unknown: highway==22 AND vfScale==0 (Spec Invariant I9).
		if hw == 22 && vfRaw == 0 {
			vfUnknownM[i] = true
		}
	}

	// Build CoordRanges from match arrays.
	result := AllRanges{
		Forest:    contiguousRangesFromMatch(forestM, edgeCoordStart, lastCoordIdx),
		Protected: contiguousRangesFromMatch(protectedM, edgeCoordStart, lastCoordIdx),
		VfUnknown: contiguousRangesFromMatch(vfUnknownM, edgeCoordStart, lastCoordIdx),
	}

	for k, level := range sacLevels {
		if r := contiguousRangesFromMatch(sacM[k], edgeCoordStart, lastCoordIdx); len(r) > 0 {
			if result.Sac == nil {
				result.Sac = make(map[string][]CoordRange, 6)
			}
			result.Sac[level] = r
		}
	}
	for k, level := range mtbLevels {
		if r := contiguousRangesFromMatch(mtbM[k], edgeCoordStart, lastCoordIdx); len(r) > 0 {
			if result.Mtb == nil {
				result.Mtb = make(map[string][]CoordRange, 6)
			}
			result.Mtb[level] = r
		}
	}
	for k, level := range vfLevels {
		if r := contiguousRangesFromMatch(vfM[k], edgeCoordStart, lastCoordIdx); len(r) > 0 {
			if result.Vf == nil {
				result.Vf = make(map[string][]CoordRange, 6)
			}
			result.Vf[level] = r
		}
	}

	return result
}

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
	if len(edgeTags2) != len(edgeCoordStart) {
		return nil // defensive — slices MUST be synchronised (Phase 1 Invariant)
	}
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
	if len(edgeTags2) != len(edgeCoordStart) {
		return nil // defensive — slices MUST be synchronised (Phase 1 Invariant)
	}
	matches := make([]bool, len(edgeTags2))
	for i, t2 := range edgeTags2 {
		matches[i] = graph.DecodeProtectedArea(t2) >= protectedHighlightMin
	}
	return contiguousRangesFromMatch(matches, edgeCoordStart, lastCoordIdx)
}

// computeVfRanges baut CoordRanges für Edges mit einem spezifischen Via-Ferrata-
// Level ("K1".."K6"). Liest Bits 33-35 aus Tags2 via graph.DecodeViaFerrataScale.
// Unbekannte Level oder leere Strings → nil.
func computeVfRanges(
	edgeCoordStart []int,
	edgeTags2 []uint64,
	lastCoordIdx int,
	level string,
) []CoordRange {
	if len(edgeTags2) != len(edgeCoordStart) {
		return nil // defensive — slices MUST be synchronised (Phase 1 Invariant)
	}
	matches := make([]bool, len(edgeTags2))
	for i, t2 := range edgeTags2 {
		raw := graph.DecodeViaFerrataScale(t2)
		if graph.ViaFerrataScaleShort(raw) == level {
			matches[i] = true
		}
	}
	return contiguousRangesFromMatch(matches, edgeCoordStart, lastCoordIdx)
}

// computeVfUnknownRanges baut CoordRanges für Edges wo highway == via_ferrata (22)
// UND vfScale == 0. Das sind die K?-Ranges (Spec-Invariant I9: IMMER auf der
// Karte sichtbar, unabhängig vom Scale-Pin).
func computeVfUnknownRanges(
	edgeCoordStart []int,
	edgeTags1 []uint64,
	edgeTags2 []uint64,
	lastCoordIdx int,
) []CoordRange {
	n := len(edgeTags1)
	if len(edgeTags2) != n || len(edgeCoordStart) != n {
		return nil // defensive — alle drei Slices MÜSSEN synchron sein (Phase 1 Invariant)
	}
	matches := make([]bool, n)
	for i := range edgeTags1 {
		hw := graph.DecodeHighway(edgeTags1[i])
		vf := graph.DecodeViaFerrataScale(edgeTags2[i])
		if hw == 22 && vf == 0 {
			matches[i] = true
		}
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
	if len(edgeTags1) != len(edgeCoordStart) {
		return nil // defensive — Slices MÜSSEN synchron sein (Phase 1 Invariant)
	}
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
