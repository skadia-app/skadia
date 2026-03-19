// distance.go contains the A* hot-path distance function.
// Three distance implementations coexist for different precision/performance tradeoffs:
//   - equirectDist (this file): int32 coords, float32 result, Bhaskara cos approx.
//     Used in A* heuristic (~500k calls/route). ~2ns/call. Precision: ±0.3% at DACH latitudes.
//   - graph.equirectDistCm (dp.go): int32 coords, int result, math.Cos.
//     Used at build-time only (DP + ElevSegDist). Precision: ±0.1%. Not hot-path.
//   - geo.DistanceHaversine: float64 coords, float64 result, full haversine.
//     Used in retrack total distance. Precision: ±0.01%. ~15ns/call.
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
// Accuracy: <0.2% error for |x| < 1.0 rad (latitudes up to ~57°).
func cosApprox(x float32) float32 {
	// Bhaskara: cos(x) ≈ (pi²-4x²) / (pi²+x²)
	const pi2 = math.Pi * math.Pi
	x2 := float64(x * x)
	return float32((pi2 - 4*x2) / (pi2 + x2))
}
