package importer

import (
	"math"

	"github.com/paulmach/orb"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// LatLng is a geographic coordinate pair.
type LatLng struct {
	Lat float64
	Lng float64
}

// latLngToLineString converts a slice of LatLng to an orb.LineString.
func latLngToLineString(pts []LatLng) orb.LineString {
	return geo.LineStringFromLatLng(pts, func(p LatLng) (float64, float64) {
		return p.Lat, p.Lng
	})
}

// GPSSimilarityPercent computes the GPS similarity between two tracks as a
// percentage (0–100). Higher = more similar.
//
// Simplifies both tracks using RDP (threshold=20m), then computes the
// Hausdorff distance. Maps distance to percentage:
//
//	0 m    → 100%
//	500 m  → 50%
//	1000 m → 0%
func GPSSimilarityPercent(a, b []LatLng) float64 {
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	lsA := geo.SimplifyDouglasPeucker(latLngToLineString(a), 20)
	lsB := geo.SimplifyDouglasPeucker(latLngToLineString(b), 20)
	h := geo.HausdorffDistance(lsA, lsB)
	pct := 100 * (1 - h/1000)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// subsample returns at most maxN evenly-spaced points from pts.
func subsample(pts []LatLng, maxN int) []LatLng {
	if len(pts) <= maxN {
		return pts
	}
	step := float64(len(pts)) / float64(maxN)
	out := make([]LatLng, 0, maxN)
	for i := range maxN {
		out = append(out, pts[int(float64(i)*step)])
	}
	return out
}

// overlapDirection computes the fraction (0–100) of points in "from" that have
// at least one point in "to" within radiusM metres.
func overlapDirection(from, to []LatLng, radiusM float64) float64 {
	covered := 0
	for _, pf := range from {
		for _, pt := range to {
			if geo.DistanceHaversine(pf.Lat, pf.Lng, pt.Lat, pt.Lng) <= radiusM {
				covered++
				break
			}
		}
	}
	return float64(covered) / float64(len(from)) * 100
}

// GPSOverlapPercent computes the percentage of track overlap between two GPS
// tracks. For each point in track A it checks whether any point in track B is
// within radiusM metres (and vice versa). Returns max(A→B, B→A).
// This is more robust than Hausdorff distance for partial overlaps (e.g. same
// tour but one device records an extra detour — a single outlier point 500m
// away kills the Hausdorff score while overlap stays high).
func GPSOverlapPercent(a, b []LatLng, radiusM float64) float64 {
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	if radiusM <= 0 {
		radiusM = 50
	}

	// Subsample both tracks to max ~300 points for performance
	sa := subsample(a, 300)
	sb := subsample(b, 300)

	coveredA := overlapDirection(sa, sb, radiusM)
	coveredB := overlapDirection(sb, sa, radiusM)

	// Return max — asymmetric: if 82% of A is in B, it's a candidate
	// even if only 75% of B is in A (B has extra segments)
	return math.Max(coveredA, coveredB)
}
