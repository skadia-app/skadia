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

// haversineDistance returns the distance in metres between two LatLng points
// using the Haversine formula.
func haversineDistance(a, b LatLng) float64 {
	const earthRadiusM = 6371000.0
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	lat1 := a.Lat * math.Pi / 180
	lat2 := b.Lat * math.Pi / 180
	sinLat := math.Sin(dLat / 2)
	sinLng := math.Sin(dLng / 2)
	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLng*sinLng
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

// subsample returns at most maxN evenly-spaced points from pts.
func subsample(pts []LatLng, maxN int) []LatLng {
	if len(pts) <= maxN {
		return pts
	}
	step := float64(len(pts)) / float64(maxN)
	out := make([]LatLng, 0, maxN)
	for i := 0; i < maxN; i++ {
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
			if haversineDistance(pf, pt) <= radiusM {
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
