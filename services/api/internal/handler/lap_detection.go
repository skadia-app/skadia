package handler

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const (
	lapGridCellSizeM    = 25.0  // spatial hash grid cell size
	lapMinDistanceM     = 100.0 // minimum lap distance
	lapMinTimeS         = 20    // minimum lap time
	lapMedianTolerance  = 0.50  // ±50% of median distance
	lapMinLaps          = 2     // minimum number of laps for detection
	lapHotspotRadiusCells = 1   // 3x3 neighborhood (center ± 1)
)

// gridCell is a spatial hash grid cell key.
type gridCell struct {
	x, y int
}

// lapBoundary marks a passage through the hotspot area.
type lapBoundary struct {
	firstIdx int // first stream index in this passage
	lastIdx  int // last stream index in this passage
}

// DetectLaps automatically detects laps/rounds within an activity's GPS stream.
// Algorithm:
//  1. Build spatial hash grid (25m cells)
//  2. Find most-visited cell (= likely start/finish area)
//  3. Cluster passages through the hotspot (3x3 neighborhood)
//  4. Extract laps between consecutive passages
//  5. Validate: min 2 laps, min distance, min time, similar lengths
func DetectLaps(ctx context.Context, db *database.DB, activityID string, gpsStream []database.GPSStreamPoint) []model.DetectedLap {
	if len(gpsStream) < 20 {
		return nil
	}

	// 1. Build spatial hash grid
	grid := buildSpatialGrid(gpsStream)
	if len(grid) == 0 {
		return nil
	}

	// 2. Find most-visited cell
	hotspot := findMostVisitedCell(grid)
	if grid[hotspot] < 3 {
		// Need at least 3 visits (2 laps = enter, leave, enter again)
		return nil
	}

	// 3. Find passages through hotspot (3x3 neighborhood)
	boundaries := extractLapBoundaries(gpsStream, hotspot)
	if len(boundaries) < lapMinLaps+1 {
		// Need at least 3 passages for 2 laps
		return nil
	}

	// 4. Extract laps between consecutive passages
	type rawLap struct {
		startIdx int
		endIdx   int
	}
	var rawLaps []rawLap
	for i := 0; i < len(boundaries)-1; i++ {
		startIdx := boundaries[i].lastIdx + 1
		endIdx := boundaries[i+1].firstIdx - 1
		if endIdx > startIdx+2 {
			rawLaps = append(rawLaps, rawLap{startIdx: startIdx, endIdx: endIdx})
		}
	}

	if len(rawLaps) < lapMinLaps {
		return nil
	}

	// 5. Compute distances and validate
	distances := make([]float64, len(rawLaps))
	for i, rl := range rawLaps {
		distances[i] = estimateSubTrackLength(gpsStream[rl.startIdx : rl.endIdx+1])
	}

	// Filter by minimum distance and time
	var validLaps []rawLap
	var validDistances []float64
	for i, rl := range rawLaps {
		elapsed := gpsStream[rl.endIdx].Time.Sub(gpsStream[rl.startIdx].Time).Seconds()
		if distances[i] >= lapMinDistanceM && elapsed >= float64(lapMinTimeS) {
			validLaps = append(validLaps, rl)
			validDistances = append(validDistances, distances[i])
		}
	}

	if len(validLaps) < lapMinLaps {
		return nil
	}

	// Filter by median distance tolerance
	medianDist := median(validDistances)
	minDist := medianDist * (1.0 - lapMedianTolerance)
	maxDist := medianDist * (1.0 + lapMedianTolerance)

	var filteredLaps []rawLap
	for i, rl := range validLaps {
		if validDistances[i] >= minDist && validDistances[i] <= maxDist {
			filteredLaps = append(filteredLaps, rl)
		}
	}

	if len(filteredLaps) < lapMinLaps {
		return nil
	}

	// 6. Build result with metrics
	laps := make([]model.DetectedLap, 0, len(filteredLaps))
	for i, rl := range filteredLaps {
		sub := gpsStream[rl.startIdx : rl.endIdx+1]
		startTime := sub[0].Time
		endTime := sub[len(sub)-1].Time
		elapsedS := int(endTime.Sub(startTime).Seconds())
		dist := estimateSubTrackLength(sub)

		lap := model.DetectedLap{
			ActivityID:   activityID,
			LapNumber:    i + 1,
			StartTime:    startTime,
			EndTime:      endTime,
			ElapsedTimeS: elapsedS,
			DistanceM:    &dist,
			StartLat:     &sub[0].Lat,
			StartLng:     &sub[0].Lng,
		}

		if dist > 0 && elapsedS > 0 {
			avgSpeed := dist / float64(elapsedS)
			lap.AvgSpeedMs = &avgSpeed
		}

		// Stream-based metrics (HR, power, cadence)
		if metrics, err := db.GetSegmentEffortMetrics(ctx, activityID, startTime, endTime); err == nil && metrics != nil {
			lap.AvgHeartRate = metrics.AvgHeartRate
			lap.AvgPowerW = metrics.AvgPowerW
		}

		// Ascent/descent from GPS altitude
		ascent, descent := computeAscentDescent(ctx, db, activityID, startTime, endTime)
		if ascent > 0 {
			lap.TotalAscentM = &ascent
		}
		if descent > 0 {
			lap.TotalDescentM = &descent
		}

		laps = append(laps, lap)
	}

	return laps
}

// buildSpatialGrid assigns each GPS point to a grid cell and counts visits.
func buildSpatialGrid(stream []database.GPSStreamPoint) map[gridCell]int {
	if len(stream) == 0 {
		return nil
	}

	// Use first point as reference for meter-based grid
	refLat := stream[0].Lat
	refLng := stream[0].Lng
	cosLat := math.Cos(refLat * math.Pi / 180.0)

	grid := make(map[gridCell]int)
	prevCell := gridCell{math.MinInt32, math.MinInt32}

	for _, p := range stream {
		// Convert lat/lng to approximate meters from reference
		dy := (p.Lat - refLat) * 111320.0
		dx := (p.Lng - refLng) * 111320.0 * cosLat

		cell := gridCell{
			x: int(math.Floor(dx / lapGridCellSizeM)),
			y: int(math.Floor(dy / lapGridCellSizeM)),
		}

		// Only count when entering a new cell (avoid counting slow movement in one cell)
		if cell != prevCell {
			grid[cell]++
			prevCell = cell
		}
	}

	return grid
}

// findMostVisitedCell returns the grid cell with the highest visit count.
func findMostVisitedCell(grid map[gridCell]int) gridCell {
	var best gridCell
	bestCount := 0
	for cell, count := range grid {
		if count > bestCount {
			bestCount = count
			best = cell
		}
	}
	return best
}

// extractLapBoundaries finds passages through the hotspot (3x3 neighborhood).
// Each passage is a contiguous group of stream indices within the hotspot.
func extractLapBoundaries(stream []database.GPSStreamPoint, hotspot gridCell) []lapBoundary {
	if len(stream) == 0 {
		return nil
	}

	refLat := stream[0].Lat
	refLng := stream[0].Lng
	cosLat := math.Cos(refLat * math.Pi / 180.0)

	// Find all indices within hotspot neighborhood
	var inHotspot []int
	for i, p := range stream {
		dy := (p.Lat - refLat) * 111320.0
		dx := (p.Lng - refLng) * 111320.0 * cosLat
		cell := gridCell{
			x: int(math.Floor(dx / lapGridCellSizeM)),
			y: int(math.Floor(dy / lapGridCellSizeM)),
		}

		if abs(cell.x-hotspot.x) <= lapHotspotRadiusCells &&
			abs(cell.y-hotspot.y) <= lapHotspotRadiusCells {
			inHotspot = append(inHotspot, i)
		}
	}

	if len(inHotspot) == 0 {
		return nil
	}

	// Cluster consecutive indices (gap > 3 points = separate passage)
	var boundaries []lapBoundary
	first := inHotspot[0]
	last := inHotspot[0]
	for i := 1; i < len(inHotspot); i++ {
		if inHotspot[i]-last > 3 {
			boundaries = append(boundaries, lapBoundary{firstIdx: first, lastIdx: last})
			first = inHotspot[i]
		}
		last = inHotspot[i]
	}
	boundaries = append(boundaries, lapBoundary{firstIdx: first, lastIdx: last})

	return boundaries
}

// computeAscentDescent queries altitude stream data and computes total ascent/descent.
func computeAscentDescent(ctx context.Context, db *database.DB, activityID string, startTime, endTime time.Time) (float64, float64) {
	rows, err := db.Pool.Query(ctx, `
		SELECT altitude_m FROM activity_streams
		WHERE activity_id = $1 AND "time" >= $2 AND "time" <= $3
		  AND altitude_m IS NOT NULL
		ORDER BY "time" ASC`, activityID, startTime, endTime)
	if err != nil {
		return 0, 0
	}
	defer rows.Close()

	var altitudes []float64
	for rows.Next() {
		var alt float64
		if err := rows.Scan(&alt); err == nil {
			altitudes = append(altitudes, alt)
		}
	}

	if len(altitudes) < 2 {
		return 0, 0
	}

	var ascent, descent float64
	for i := 1; i < len(altitudes); i++ {
		diff := altitudes[i] - altitudes[i-1]
		if diff > 0 {
			ascent += diff
		} else {
			descent -= diff
		}
	}
	return ascent, descent
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
