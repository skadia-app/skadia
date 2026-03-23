package geo

import (
	"math"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/simplify"
)

// SimplifyDouglasPeucker reduces the number of points in a LineString using
// the Douglas-Peucker algorithm with a threshold given in meters.
//
// The underlying orb/simplify package operates in planar degrees², not geodesic
// meters. This wrapper converts thresholdMeters to a corrected degree threshold
// using the cosine of the center latitude of the LineString.
//
// Limitation: the cosine correction uses a single center latitude, which may
// be inaccurate for tracks spanning more than ~200 km north-south.
//
// Returns the input unchanged if it has fewer than 3 points.
func SimplifyDouglasPeucker(ls orb.LineString, thresholdMeters float64) orb.LineString {
	if len(ls) < 3 {
		return ls
	}
	centerLat := centerLatitude(ls)
	thresholdDeg := thresholdMeters / MetersPerDegreeLat
	cosLat := math.Cos(centerLat * math.Pi / 180.0)
	if cosLat > 0.01 {
		thresholdDeg = thresholdDeg / math.Sqrt(cosLat)
	}
	return simplify.DouglasPeucker(thresholdDeg).LineString(ls)
}

// centerLatitude returns the mean latitude of all points in the LineString.
// orb.Point is [lng, lat], so index [1] is latitude.
func centerLatitude(ls orb.LineString) float64 {
	if len(ls) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range ls {
		sum += p[1]
	}
	return sum / float64(len(ls))
}
