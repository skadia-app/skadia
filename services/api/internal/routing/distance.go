package routing

import "math"

// equirectDist calculates equirectangular approximation distance in meters.
// Input coordinates are fixed-point int32 (value * 1e7).
// Accuracy: <0.3% error at DACH latitudes (46-55°N).
func equirectDist(lat1, lon1, lat2, lon2 int32) float32 {
	const degToRad = 1.745329252e-9 // pi / (180 * 1e7)
	const earthR = 6_371_000.0

	dlat := float32(lat2-lat1) * degToRad
	dlon := float32(lon2-lon1) * degToRad
	midLat := float32(lat1+lat2) / 2.0 * degToRad
	cosLat := cosApprox(midLat)

	dx := dlon * cosLat
	dy := dlat
	return earthR * float32(math.Sqrt(float64(dx*dx+dy*dy)))
}

// cosApprox computes cos(x) using Bhaskara I's approximation.
// Accuracy: <0.2% error for |x| < pi/2 (covers all Earth latitudes).
func cosApprox(x float32) float32 {
	// Bhaskara: cos(x) ≈ (pi²-4x²) / (pi²+x²)
	const pi2 = math.Pi * math.Pi
	x2 := float64(x * x)
	return float32((pi2 - 4*x2) / (pi2 + x2))
}
