package graph

import (
	"log/slog"

	"github.com/skadia-app/skadia/services/api/internal/dem"
)

// SampleElevationFromReader queries elevations using a dem.Reader.
// Returns elevations in decimeters. NoData values are replaced with 0.
func SampleElevationFromReader(reader dem.Reader, lats, lons []int32,
	progressFn func(done, total int)) []int32 {

	n := len(lats)
	elevations := reader.ElevationBatch(lats, lons)

	noDataCount := 0
	for i, v := range elevations {
		if v == dem.NoData {
			elevations[i] = 0
			noDataCount++
		}
	}
	if noDataCount > 0 {
		slog.Info("DEM no-data points replaced with 0",
			"count", noDataCount, "total", n)
	}

	if progressFn != nil {
		progressFn(n, n)
	}
	return elevations
}

// SampleEdgeElevationProfilesFromReader fills edge elevation profiles using a dem.Reader.
func SampleEdgeElevationProfilesFromReader(reader dem.Reader, g *Graph) {
	totalGeomPoints := len(g.EdgeGeomLat)
	if totalGeomPoints == 0 {
		for i := uint32(0); i <= g.EdgeCount; i++ {
			g.EdgeElevOff[i] = 0
		}
		return
	}

	elevations := reader.ElevationBatch(g.EdgeGeomLat, g.EdgeGeomLon)

	for i, v := range elevations {
		if v == dem.NoData {
			elevations[i] = 0
		}
	}

	g.EdgeElevProf = make([]int32, totalGeomPoints)
	copy(g.EdgeElevProf, elevations)
	copy(g.EdgeElevOff, g.EdgeGeomOff)
}

// InterpolateMissing fills gaps in elevation data for bridge/tunnel segments.
func InterpolateMissing(elevations []int32, isBridge []bool) {
	n := len(elevations)
	if n == 0 {
		return
	}

	i := 0
	for i < n {
		if !isBridge[i] || elevations[i] != 0 {
			i++
			continue
		}

		start := i
		for i < n && isBridge[i] && elevations[i] == 0 {
			i++
		}
		end := i

		var leftVal, rightVal int32
		if start > 0 {
			leftVal = elevations[start-1]
		}
		if end < n {
			rightVal = elevations[end]
		}

		if start == 0 && end < n {
			leftVal = rightVal
		} else if end == n && start > 0 {
			rightVal = leftVal
		} else if start == 0 && end == n {
			continue
		}

		for j := start; j < end; j++ {
			if start == 0 {
				elevations[j] = leftVal
			} else {
				pos := j - start + 1
				frac := float64(pos) / float64(end-start+1)
				elevations[j] = leftVal + int32(float64(rightVal-leftVal)*frac)
			}
		}
	}

	interpolated := 0
	for i, b := range isBridge {
		if b && elevations[i] != 0 {
			interpolated++
		}
	}
	if interpolated > 0 {
		slog.Info("interpolated bridge/tunnel elevations", "count", interpolated)
	}
}
