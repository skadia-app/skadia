package geo

import (
	"math"

	"github.com/paulmach/orb"
	orbgeo "github.com/paulmach/orb/geo"
)

const earthRadiusPi = orb.EarthRadius * math.Pi

// MetersPerDegreeLat is the approximate number of meters per degree of latitude.
const MetersPerDegreeLat = 111320.0

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

// PointToMercator projects a single WGS84 point (lng, lat) to Spherical
// Pseudo-Mercator (EPSG:3857) where X and Y are in meters.
// Matches PostGIS ST_Transform(..., 3857).
func PointToMercator(p orb.Point) orb.Point {
	y := math.Log(math.Tan((90.0+p[1])*math.Pi/360.0)) * orb.EarthRadius
	return orb.Point{
		earthRadiusPi / 180.0 * p[0],
		max(-earthRadiusPi, min(y, earthRadiusPi)),
	}
}

// BearingDeg returns the initial bearing in degrees (0–360, clockwise from north)
// from (lat1, lon1) to (lat2, lon2) using WGS84 coordinates.
func BearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	lat1r := lat1 * math.Pi / 180
	lat2r := lat2 * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	x := math.Sin(dLon) * math.Cos(lat2r)
	y := math.Cos(lat1r)*math.Sin(lat2r) - math.Sin(lat1r)*math.Cos(lat2r)*math.Cos(dLon)
	bearing := math.Atan2(x, y) * 180 / math.Pi
	return math.Mod(bearing+360, 360)
}

// LineStringToMercator projects an entire WGS84 LineString to Spherical
// Pseudo-Mercator. Returns a NEW LineString (does not modify the input).
func LineStringToMercator(ls orb.LineString) orb.LineString {
	out := make(orb.LineString, len(ls))
	for i, p := range ls {
		out[i] = PointToMercator(p)
	}
	return out
}
