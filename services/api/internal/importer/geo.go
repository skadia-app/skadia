package importer

import "github.com/skadia-app/skadia/services/api/internal/geo"

// HaversineM returns the great-circle distance in meters between two WGS-84 points.
// Delegates to geo.DistanceHaversine.
func HaversineM(lat1, lon1, lat2, lon2 float64) float64 {
	return geo.DistanceHaversine(lat1, lon1, lat2, lon2)
}
