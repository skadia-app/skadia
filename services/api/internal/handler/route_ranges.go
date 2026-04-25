package handler

import (
	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

type CoordRange = edgetags.CoordRange
type AllRanges = edgetags.AllRanges

const (
	forestHighlightMin    = edgetags.ForestHighlightMin
	protectedHighlightMin = edgetags.ProtectedHighlightMin
)

var (
	sacLevels = edgetags.SacLevels
	mtbLevels = edgetags.MtbLevels
	vfLevels  = edgetags.VfLevels
)

func computeAllRanges(
	edgeCoordStart []int,
	edgeTags1 []uint64,
	edgeTags2 []uint64,
	lastCoordIdx int,
) AllRanges {
	return edgetags.ComputeAllRanges(edgeCoordStart, edgeTags1, edgeTags2, lastCoordIdx)
}

func contiguousRangesFromMatch(
	matches []bool,
	edgeCoordStart []int,
	lastCoordIdx int,
) []CoordRange {
	return edgetags.ContiguousRangesFromMatch(matches, edgeCoordStart, lastCoordIdx)
}

func lastRealCoordIdx(r *routing.RouteResult) int {
	return edgetags.LastRealCoordIdx(len(r.Coordinates), r.LastEdgeCoordEnd)
}

func aggregateForestProtectedFromResult(r *routing.RouteResult) (forestPct, protectedPct float64) {
	coords := make([]edgetags.LatLng, len(r.Coordinates))
	for i, c := range r.Coordinates {
		coords[i] = edgetags.LatLng{Lat: c.Lat, Lng: c.Lon}
	}
	return edgetags.AggregateForestProtected(
		coords,
		r.EdgeCoordStart,
		r.EdgeTags2,
		edgetags.LastRealCoordIdx(len(r.Coordinates), r.LastEdgeCoordEnd),
	)
}
