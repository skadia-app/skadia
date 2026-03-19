package geo

import (
	"math"

	"github.com/paulmach/orb"
)

// HausdorffDistance returns the discrete Hausdorff distance in meters between
// two WGS84 LineStrings using the Haversine formula.
// Returns math.Inf(1) if either LineString is empty.
func HausdorffDistance(a, b orb.LineString) float64 {
	if len(a) == 0 || len(b) == 0 {
		return math.Inf(1)
	}
	return max(directedHausdorff(a, b), directedHausdorff(b, a))
}

// HausdorffDistancePlanar returns the discrete Hausdorff distance using
// Euclidean distance. Use this when coordinates are already in a projected
// coordinate system (e.g. Mercator) where X/Y values are in meters.
// Returns math.Inf(1) if either LineString is empty.
func HausdorffDistancePlanar(a, b orb.LineString) float64 {
	if len(a) == 0 || len(b) == 0 {
		return math.Inf(1)
	}
	return max(directedHausdorffPlanar(a, b), directedHausdorffPlanar(b, a))
}

// directedHausdorff computes max over all points in a of (min Haversine distance to b).
func directedHausdorff(a, b orb.LineString) float64 {
	maxDist := 0.0
	for _, pa := range a {
		minDist := math.Inf(1)
		for _, pb := range b {
			d := DistanceHaversine(pa[1], pa[0], pb[1], pb[0])
			if d < minDist {
				minDist = d
			}
		}
		if minDist > maxDist {
			maxDist = minDist
		}
	}
	return maxDist
}

// directedHausdorffPlanar computes max over all points in a of (min Euclidean distance to b).
func directedHausdorffPlanar(a, b orb.LineString) float64 {
	maxDist := 0.0
	for _, pa := range a {
		minDist := math.Inf(1)
		for _, pb := range b {
			dx := pa[0] - pb[0]
			dy := pa[1] - pb[1]
			d := math.Sqrt(dx*dx + dy*dy)
			if d < minDist {
				minDist = d
			}
		}
		if minDist > maxDist {
			maxDist = minDist
		}
	}
	return maxDist
}
