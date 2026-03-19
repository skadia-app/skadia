// dp.go
package graph

import "math"

const dpDegToRad = math.Pi / 180.0 / 1e7 // int32 fixed-point → radians
const dpEarthR = 6_371_000.0             // meters

// DouglasPeucker returns a keep-mask for geometry points.
// Start and end points always kept. Uses equirectangular perpendicular
// distance (cosLat-corrected). Threshold in meters.
// BRouter-style 2D DP — elevation profile kept separately at full resolution.
func DouglasPeucker(lats, lons []int32, cosLat float64, thresholdM float64) []bool {
	n := len(lats)
	if n <= 2 {
		mask := make([]bool, n)
		for i := range mask {
			mask[i] = true
		}
		return mask
	}

	mask := make([]bool, n)
	mask[0] = true
	mask[n-1] = true

	dpRecurse(lats, lons, cosLat, thresholdM, mask, 0, n-1)
	return mask
}

func dpRecurse(lats, lons []int32, cosLat, thresholdM float64, mask []bool, start, end int) {
	if end-start < 2 {
		return
	}

	maxDist := 0.0
	maxIdx := start

	// Line from start to end
	ax := float64(lons[start]) * dpDegToRad * cosLat * dpEarthR
	ay := float64(lats[start]) * dpDegToRad * dpEarthR
	bx := float64(lons[end]) * dpDegToRad * cosLat * dpEarthR
	by := float64(lats[end]) * dpDegToRad * dpEarthR

	dx := bx - ax
	dy := by - ay
	lenSq := dx*dx + dy*dy

	for i := start + 1; i < end; i++ {
		px := float64(lons[i]) * dpDegToRad * cosLat * dpEarthR
		py := float64(lats[i]) * dpDegToRad * dpEarthR

		var dist float64
		if lenSq < 1e-10 {
			// Degenerate: start == end
			ddx := px - ax
			ddy := py - ay
			dist = math.Sqrt(ddx*ddx + ddy*ddy)
		} else {
			// Perpendicular distance to line
			t := ((px-ax)*dx + (py-ay)*dy) / lenSq
			t = max(0, min(1, t))
			projX := ax + t*dx
			projY := ay + t*dy
			ddx := px - projX
			ddy := py - projY
			dist = math.Sqrt(ddx*ddx + ddy*ddy)
		}

		if dist > maxDist {
			maxDist = dist
			maxIdx = i
		}
	}

	if maxDist >= thresholdM {
		mask[maxIdx] = true
		dpRecurse(lats, lons, cosLat, thresholdM, mask, start, maxIdx)
		dpRecurse(lats, lons, cosLat, thresholdM, mask, maxIdx, end)
	}
}

// ComputeElevSegDist computes segment distances between consecutive geometry
// points in centimeters (uint16, max 65535 = 655.35m).
// MUST be called BEFORE Douglas-Peucker (needs original coordinates).
// Returns N-1 values for N input points.
func ComputeElevSegDist(lats, lons []int32, cosLat float64) []uint16 {
	n := len(lats)
	if n <= 1 {
		return nil
	}

	dists := make([]uint16, n-1)
	for i := range n - 1 {
		dist := equirectDistCm(lats[i], lons[i], lats[i+1], lons[i+1], cosLat)
		dists[i] = uint16(min(65535, dist))
	}
	return dists
}

// equirectDistCm uses math.Cos (not Bhaskara approx) because build-time precision
// matters more than speed here. See routing/distance.go for the hot-path variant.
func equirectDistCm(lat1, lon1, lat2, lon2 int32, cosLat float64) int {
	dlat := float64(lat2-lat1) * dpDegToRad * dpEarthR
	dlon := float64(lon2-lon1) * dpDegToRad * dpEarthR * cosLat
	return int(math.Sqrt(dlat*dlat+dlon*dlon) * 100)
}

// ApplyDPToEdgeGeometry applies Douglas-Peucker to edge geometry arrays.
// Returns new (simplified) geom arrays and new geom offsets.
// Original arrays are NOT modified.
// cosLat: cos(center latitude of tile) for equirectangular projection.
func ApplyDPToEdgeGeometry(
	edgeCount uint32,
	geomOff []uint32,
	geomLat, geomLon []int32,
	cosLat float64,
	thresholdM float64,
) (newGeomOff []uint32, newGeomLat, newGeomLon []int32) {
	newGeomOff = make([]uint32, edgeCount+1)
	// Pre-estimate: ~40% survival rate after DP
	estimatedPoints := len(geomLat) * 2 / 5
	if estimatedPoints < 1024 {
		estimatedPoints = 1024
	}
	newGeomLat = make([]int32, 0, estimatedPoints)
	newGeomLon = make([]int32, 0, estimatedPoints)

	for ei := range edgeCount {
		start := geomOff[ei]
		end := geomOff[ei+1]
		nPts := end - start

		newGeomOff[ei] = uint32(len(newGeomLat))

		if nPts <= 2 {
			// Keep all points (start+end only, or single/empty)
			newGeomLat = append(newGeomLat, geomLat[start:end]...)
			newGeomLon = append(newGeomLon, geomLon[start:end]...)
			continue
		}

		edgeLats := geomLat[start:end]
		edgeLons := geomLon[start:end]
		mask := DouglasPeucker(edgeLats, edgeLons, cosLat, thresholdM)

		for j, keep := range mask {
			if keep {
				newGeomLat = append(newGeomLat, edgeLats[j])
				newGeomLon = append(newGeomLon, edgeLons[j])
			}
		}
	}
	newGeomOff[edgeCount] = uint32(len(newGeomLat))

	return newGeomOff, newGeomLat, newGeomLon
}
