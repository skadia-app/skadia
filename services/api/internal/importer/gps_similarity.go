package importer

import "math"

// LatLng is a geographic coordinate pair.
type LatLng struct {
	Lat float64
	Lng float64
}

// perpendicularDist returns the perpendicular distance from point p to the
// line segment defined by start and end.
func perpendicularDist(p, start, end LatLng) float64 {
	// Convert to approximate Cartesian (metres) centred on start.
	// 1° lat ≈ 111320 m; 1° lng ≈ 111320 * cos(lat) m
	latScale := 111320.0
	lngScale := 111320.0 * math.Cos(start.Lat*math.Pi/180)

	dx := (end.Lng - start.Lng) * lngScale
	dy := (end.Lat - start.Lat) * latScale
	px := (p.Lng - start.Lng) * lngScale
	py := (p.Lat - start.Lat) * latScale

	lenSq := dx*dx + dy*dy
	if lenSq == 0 {
		return math.Sqrt(px*px + py*py)
	}
	t := (px*dx + py*dy) / lenSq
	t = math.Max(0, math.Min(1, t))
	rx := px - t*dx
	ry := py - t*dy
	return math.Sqrt(rx*rx + ry*ry)
}

// rdpSimplify simplifies a polyline using the Ramer-Douglas-Peucker algorithm.
// epsilon is the maximum perpendicular distance in metres to keep a point.
func rdpSimplify(points []LatLng, epsilon float64) []LatLng {
	if len(points) < 3 {
		return points
	}

	// Find the point with the maximum distance from the line start→end.
	maxDist := 0.0
	maxIdx := 0
	start := points[0]
	end := points[len(points)-1]
	for i := 1; i < len(points)-1; i++ {
		d := perpendicularDist(points[i], start, end)
		if d > maxDist {
			maxDist = d
			maxIdx = i
		}
	}

	if maxDist <= epsilon {
		// All intermediate points are within epsilon — keep only endpoints.
		return []LatLng{start, end}
	}

	// Recurse on both halves.
	left := rdpSimplify(points[:maxIdx+1], epsilon)
	right := rdpSimplify(points[maxIdx:], epsilon)
	// Concatenate without duplicating the split point.
	return append(left[:len(left)-1], right...)
}

// hausdorffDistance computes the discrete Hausdorff distance (in metres) between
// two simplified GPS tracks.
func hausdorffDistance(a, b []LatLng) float64 {
	if len(a) == 0 || len(b) == 0 {
		return math.MaxFloat64
	}
	latScale := 111320.0

	dist := func(p, q LatLng) float64 {
		lngScale := 111320.0 * math.Cos(p.Lat*math.Pi/180)
		dy := (p.Lat - q.Lat) * latScale
		dx := (p.Lng - q.Lng) * lngScale
		return math.Sqrt(dx*dx + dy*dy)
	}

	// directed Hausdorff: max over a of min distance to any point in b
	directedHausdorff := func(from, to []LatLng) float64 {
		h := 0.0
		for _, p := range from {
			minD := math.MaxFloat64
			for _, q := range to {
				if d := dist(p, q); d < minD {
					minD = d
				}
			}
			if minD > h {
				h = minD
			}
		}
		return h
	}

	return math.Max(directedHausdorff(a, b), directedHausdorff(b, a))
}

// GPSSimilarityPercent computes the GPS similarity between two tracks as a
// percentage (0–100). Higher = more similar.
//
// Simplifies both tracks to ≤100 points using RDP (epsilon=20m), then
// computes the Hausdorff distance. Maps distance to percentage:
//
//	0 m    → 100%
//	500 m  → 50%
//	1000 m → 0%
func GPSSimilarityPercent(a, b []LatLng) float64 {
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	sa := rdpSimplify(a, 20)
	sb := rdpSimplify(b, 20)
	h := hausdorffDistance(sa, sb)
	pct := 100 * (1 - h/1000)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}
