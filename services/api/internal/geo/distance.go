package geo

import (
	"github.com/paulmach/orb"
	orbgeo "github.com/paulmach/orb/geo"
)

// DistanceHaversine returns the great-circle distance in meters between two
// WGS84 coordinates using the Haversine formula.
func DistanceHaversine(lat1, lng1, lat2, lng2 float64) float64 {
	return orbgeo.DistanceHaversine(orb.Point{lng1, lat1}, orb.Point{lng2, lat2})
}

// Length returns the total length in meters of a LineString using the
// Haversine formula for each segment.
func Length(ls orb.LineString) float64 {
	return orbgeo.LengthHaversine(ls)
}
