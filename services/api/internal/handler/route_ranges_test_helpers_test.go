package handler

import "github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"

func computeForestRanges(edgeCoordStart []int, edgeTags2 []uint64, lastCoordIdx int) []CoordRange {
	if len(edgeTags2) != len(edgeCoordStart) {
		return nil
	}
	zeros := make([]uint64, len(edgeTags2))
	return edgetags.ComputeAllRanges(edgeCoordStart, zeros, edgeTags2, lastCoordIdx).Forest
}

func computeProtectedRanges(edgeCoordStart []int, edgeTags2 []uint64, lastCoordIdx int) []CoordRange {
	if len(edgeTags2) != len(edgeCoordStart) {
		return nil
	}
	zeros := make([]uint64, len(edgeTags2))
	return edgetags.ComputeAllRanges(edgeCoordStart, zeros, edgeTags2, lastCoordIdx).Protected
}

func computeVfRanges(edgeCoordStart []int, edgeTags2 []uint64, lastCoordIdx int, level string) []CoordRange {
	if len(edgeTags2) != len(edgeCoordStart) {
		return nil
	}
	zeros := make([]uint64, len(edgeTags2))
	all := edgetags.ComputeAllRanges(edgeCoordStart, zeros, edgeTags2, lastCoordIdx)
	if all.Vf == nil {
		return nil
	}
	return all.Vf[level]
}

func computeVfUnknownRanges(edgeCoordStart []int, edgeTags1, edgeTags2 []uint64, lastCoordIdx int) []CoordRange {
	if len(edgeTags1) != len(edgeTags2) || len(edgeTags1) != len(edgeCoordStart) {
		return nil
	}
	return edgetags.ComputeAllRanges(edgeCoordStart, edgeTags1, edgeTags2, lastCoordIdx).VfUnknown
}

func computeScaleRanges(edgeCoordStart []int, edgeTags1 []uint64, lastCoordIdx int, kind, level string) []CoordRange {
	if len(edgeTags1) != len(edgeCoordStart) {
		return nil
	}
	zeros := make([]uint64, len(edgeTags1))
	all := edgetags.ComputeAllRanges(edgeCoordStart, edgeTags1, zeros, lastCoordIdx)
	switch kind {
	case "sac":
		if all.Sac == nil {
			return nil
		}
		return all.Sac[level]
	case "mtb":
		if all.Mtb == nil {
			return nil
		}
		return all.Mtb[level]
	default:
		return nil
	}
}
