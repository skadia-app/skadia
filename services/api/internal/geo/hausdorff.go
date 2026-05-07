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

// HausdorffDistancePlanar returns a polyline Hausdorff distance using Euclidean
// point-to-segment distance. Use this when coordinates are already in a
// projected coordinate system where X/Y values are in meters.
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

// directedHausdorffPlanar computes max over all points in a of (min Euclidean distance to b's polyline).
func directedHausdorffPlanar(a, b orb.LineString) float64 {
	maxDist := 0.0
	for _, pa := range a {
		minDist := pointToLineStringDistancePlanar(pa, b)
		if minDist > maxDist {
			maxDist = minDist
		}
	}
	return maxDist
}

func pointToLineStringDistancePlanar(p orb.Point, ls orb.LineString) float64 {
	if len(ls) == 0 {
		return math.Inf(1)
	}
	if len(ls) == 1 {
		return pointDistancePlanar(p, ls[0])
	}

	minDist := math.Inf(1)
	for i := 0; i < len(ls)-1; i++ {
		d := pointToSegmentDistancePlanar(p, ls[i], ls[i+1])
		if d < minDist {
			minDist = d
		}
	}
	return minDist
}

func pointToSegmentDistancePlanar(p, a, b orb.Point) float64 {
	dx := b[0] - a[0]
	dy := b[1] - a[1]
	if dx == 0 && dy == 0 {
		return pointDistancePlanar(p, a)
	}

	t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / (dx*dx + dy*dy)
	t = max(0, min(1, t))
	proj := orb.Point{a[0] + t*dx, a[1] + t*dy}
	return pointDistancePlanar(p, proj)
}

func pointDistancePlanar(a, b orb.Point) float64 {
	dx := a[0] - b[0]
	dy := a[1] - b[1]
	return math.Sqrt(dx*dx + dy*dy)
}
