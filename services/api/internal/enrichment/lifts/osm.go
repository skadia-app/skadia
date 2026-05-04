package lifts

import "math"

// OSM-matching constants — identical to lifts.py top-level constants.
const (
	ProximityM    = 100.0
	HausdorffMaxM = 50.0
	MinCoverage   = 0.7
)

// OSMLiftMatch holds the result of the Method-A SQL query for one OSM lift.
type OSMLiftMatch struct {
	ID            string
	OsmID         int64
	Name          string
	LiftLengthM   float64
	HausdorffM    float64
	CoverageRatio float64
	LiftBearing   float64
}

// BearingDeg returns the initial bearing in degrees [0, 360) from point
// (lat1, lon1) to (lat2, lon2). Ported 1:1 from lifts.py:231-236 (_bearing).
func BearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	lat1r := math.Pi / 180 * lat1
	lat2r := math.Pi / 180 * lat2
	dlonr := math.Pi / 180 * (lon2 - lon1)
	x := math.Sin(dlonr) * math.Cos(lat2r)
	y := math.Cos(lat1r)*math.Sin(lat2r) - math.Sin(lat1r)*math.Cos(lat2r)*math.Cos(dlonr)
	return math.Mod(math.Atan2(x, y)*180/math.Pi+360, 360)
}

// IsReverseHeading reports whether the track bearing indicates travel in the
// reverse direction of the lift. Returns true when the angular difference
// exceeds 90°. Ported 1:1 from lifts.py:239-243 (_check_is_reverse).
func IsReverseHeading(trackBearing, liftBearing float64) bool {
	diff := math.Abs(trackBearing-liftBearing)
	diff = math.Mod(diff, 360)
	if diff > 180 {
		diff = 360 - diff
	}
	return diff > 90
}

// Haversine returns the great-circle distance in metres between two WGS-84
// coordinates. Ported 1:1 from lifts.py:367-373 (_haversine).
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	phi1 := math.Pi / 180 * lat1
	phi2 := math.Pi / 180 * lat2
	dphi := math.Pi / 180 * (lat2 - lat1)
	dlam := math.Pi / 180 * (lon2 - lon1)
	a := math.Sin(dphi/2)*math.Sin(dphi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dlam/2)*math.Sin(dlam/2)
	return 2 * R * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

