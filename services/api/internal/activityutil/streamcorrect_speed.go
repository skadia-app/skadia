package activityutil

import (
	"math"
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// speedPointInfo holds per-point computed data for the speed plausibility filter.
type speedPointInfo struct {
	index       int
	speed       float64 // GPS-derived speed in m/s
	gradient    float64 // gradient as fraction (negative = downhill)
	hasCadence  bool
	maxAllowed  float64 // computed max speed (base + gradient + cadence boosts)
	accel       float64 // acceleration from previous point (informational)
}

// CorrectSpeedOutliers detects and corrects implausible GPS-derived speed values
// that pass the spatial outlier detector. It uses gradient-aware speed thresholds
// per sport group with sensor correlation (cadence) to minimize false positives.
//
// The algorithm:
//  1. Resolve base max speed from sport group / per-sport override.
//  2. Compute per-point speed, gradient, cadence, and maxAllowed.
//  3. Flag candidates exceeding maxAllowed * MinOvershootRatio.
//  4. Remove consecutive runs longer than MaxConsecutiveOver (gondola/shuttle).
//  5. Apply cascading protection: correcting idx removes idx+1 from candidates.
//  6. Interpolate corrected points as midpoint of neighbors.
func CorrectSpeedOutliers(points []model.StreamPoint, sportType string, cfg SpeedCorrectionConfig) ([]model.StreamPoint, SpeedCorrectionReport) {
	report := SpeedCorrectionReport{TotalPoints: len(points)}

	if len(points) < cfg.MinPointsRequired {
		return points, report
	}

	n := len(points)

	// ── Step 1: Resolve base max speed ────────────────────────────────────
	group := sport.SegmentGroup(sportType)
	baseMax := cfg.MaxSpeedByGroup[group]
	if baseMax <= 0 {
		baseMax = cfg.MaxSpeedByGroup["generic"]
	}
	if perSport := sport.MaxSpeedFlatForSport(sportType); perSport > 0 {
		baseMax = perSport
	}

	// ── Step 2: Compute per-point info ────────────────────────────────────
	infos := make([]speedPointInfo, n)
	var prevSpeed float64

	for i := 1; i < n; i++ {
		if points[i].Lat == nil || points[i].Lng == nil {
			continue
		}
		if points[i-1].Lat == nil || points[i-1].Lng == nil {
			continue
		}

		dt := points[i].Time.Sub(points[i-1].Time).Seconds()
		if dt <= 0 || dt > speedGapMaxSeconds {
			continue
		}

		dist := HaversineMeters(*points[i-1].Lat, *points[i-1].Lng, *points[i].Lat, *points[i].Lng)
		speed := dist / dt

		// Gradient computation.
		var gradient float64
		if points[i].AltitudeM != nil && points[i-1].AltitudeM != nil && dist > 1.0 {
			gradient = (*points[i].AltitudeM - *points[i-1].AltitudeM) / dist
		}

		// Downhill bonus: add extra allowed speed for downhill gradient.
		var downhillBonus float64
		if gradient < 0 {
			downhillBonus = cfg.GradientBonusFactor * math.Abs(gradient) * 100
		}

		maxAllowed := baseMax + downhillBonus

		// Cadence boost.
		hasCadence := points[i].Cadence != nil && *points[i].Cadence > 0
		if hasCadence {
			maxAllowed *= cfg.CadenceBoostFactor
		}

		// Acceleration (informational).
		var accel float64
		if prevSpeed > 0 && dt > 0 {
			accel = math.Abs(speed-prevSpeed) / dt
		}
		prevSpeed = speed

		infos[i] = speedPointInfo{
			index:      i,
			speed:      speed,
			gradient:   gradient,
			hasCadence: hasCadence,
			maxAllowed: maxAllowed,
			accel:      accel,
		}
	}

	// ── Step 3: Flag candidates ───────────────────────────────────────────
	// Only interior points (1..n-2) that have GPS neighbors for interpolation.
	candidateSet := make(map[int]speedPointInfo)
	for i := 1; i < n-1; i++ {
		info := infos[i]
		if info.speed <= 0 {
			continue
		}
		if info.speed <= info.maxAllowed*cfg.MinOvershootRatio {
			continue
		}
		// Verify GPS neighbors exist for interpolation.
		if points[i-1].Lat == nil || points[i-1].Lng == nil {
			continue
		}
		if points[i+1].Lat == nil || points[i+1].Lng == nil {
			continue
		}
		candidateSet[i] = info
	}

	report.CandidateCount = len(candidateSet)

	// ── Step 4: Consecutive-run filter ────────────────────────────────────
	if len(candidateSet) > 0 {
		indices := make([]int, 0, len(candidateSet))
		for idx := range candidateSet {
			indices = append(indices, idx)
		}
		sort.Ints(indices)

		// Find runs of consecutive indices.
		runStart := 0
		for i := 1; i <= len(indices); i++ {
			if i < len(indices) && indices[i] == indices[i-1]+1 {
				continue
			}
			// End of run: indices[runStart..i-1]
			runLen := i - runStart
			if runLen > cfg.MaxConsecutiveOver {
				for j := runStart; j < i; j++ {
					info := candidateSet[indices[j]]
					report.SkippedConsec++
					report.Points = append(report.Points, SpeedPointCorrection{
						Index:        info.index,
						SpeedMS:      info.speed,
						MaxAllowedMS: info.maxAllowed,
						GradientPct:  info.gradient * 100,
						HasCadence:   info.hasCadence,
						AccelMS2:     info.accel,
						Corrected:    false,
					})
					delete(candidateSet, indices[j])
				}
			}
			runStart = i
		}
	}

	// ── Step 5+6: Cascading protection + apply corrections ────────────────
	corrIndices := make([]int, 0, len(candidateSet))
	for idx := range candidateSet {
		corrIndices = append(corrIndices, idx)
	}
	sort.Ints(corrIndices)

	// Build a set for fast removal during cascading protection.
	corrSet := make(map[int]bool, len(corrIndices))
	for _, idx := range corrIndices {
		corrSet[idx] = true
	}

	for _, idx := range corrIndices {
		if !corrSet[idx] {
			// Was removed by cascading protection from a previous correction.
			continue
		}

		info := candidateSet[idx]

		// Cascading protection: remove idx+1 from correction set if present.
		// Its speed was measured from the displaced position and will change
		// after this point is interpolated.
		if corrSet[idx+1] {
			// Record the skipped neighbor in the report.
			neighborInfo := candidateSet[idx+1]
			report.Points = append(report.Points, SpeedPointCorrection{
				Index:        neighborInfo.index,
				SpeedMS:      neighborInfo.speed,
				MaxAllowedMS: neighborInfo.maxAllowed,
				GradientPct:  neighborInfo.gradient * 100,
				HasCadence:   neighborInfo.hasCadence,
				AccelMS2:     neighborInfo.accel,
				Corrected:    false,
			})
			delete(corrSet, idx+1)
		}

		// For speed outliers (unlike spatial spikes), the position lies on the
		// correct trajectory but is too far from the previous point. Midpoint
		// interpolation doesn't help because the neighbors are also displaced.
		// Instead: move the point closer to the previous point along the bearing,
		// capping the distance to maxAllowed * dt.
		prevLat := *points[idx-1].Lat
		prevLng := *points[idx-1].Lng
		currLat := *points[idx].Lat
		currLng := *points[idx].Lng

		dt := points[idx].Time.Sub(points[idx-1].Time).Seconds()
		if dt <= 0 {
			dt = 1
		}
		dist := HaversineMeters(prevLat, prevLng, currLat, currLng)
		maxDist := info.maxAllowed * dt // max plausible distance in this interval

		if dist > maxDist && dist > 0 {
			// Store original coordinates — only if not already set by spatial filter.
			if points[idx].RawLat == nil {
				points[idx].RawLat = &currLat
				points[idx].RawLng = &currLng
			}

			// Move point along the bearing from prev to curr, but cap at maxDist.
			ratio := maxDist / dist
			newLat := prevLat + (currLat-prevLat)*ratio
			newLng := prevLng + (currLng-prevLng)*ratio
			points[idx].Lat = &newLat
			points[idx].Lng = &newLng

			// Clear derived fields — FillMissingDistanceAndSpeed will recompute.
			points[idx].SpeedMS = nil
			points[idx].DistanceM = nil
		}

		report.CorrectedCount++
		report.Points = append(report.Points, SpeedPointCorrection{
			Index:        info.index,
			SpeedMS:      info.speed,
			MaxAllowedMS: info.maxAllowed,
			GradientPct:  info.gradient * 100,
			HasCadence:   info.hasCadence,
			AccelMS2:     info.accel,
			Corrected:    true,
		})
	}

	return points, report
}
