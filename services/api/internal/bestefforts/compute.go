package bestefforts

import (
	"math"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// EffortResult holds the computed best effort for a single category.
type EffortResult struct {
	Category       string
	ElapsedTimeS   int
	ValueNumeric   float64
	StreamStartIdx int
	StreamEndIdx   int
	Suspicious     bool
}

// StuckRegion describes a contiguous segment of stream indices where power
// was identical (within 0.5 W) for longer than the minimum stuck duration.
type StuckRegion struct {
	StartIdx int
	EndIdx   int
}

// findStuckPowerRegions scans the entire stream for regions where power values
// are identical for >minDuration seconds AND value > threshold × medianPower.
// Uses timestamps (not sample count) for Smart Recording compatibility.
func findStuckPowerRegions[T model.BestEffortPoint](stream []T, medianPower float64, minDurationS float64, thresholdMultiplier float64) []StuckRegion {
	if len(stream) == 0 || medianPower <= 0 {
		return nil
	}
	var regions []StuckRegion
	runStart := 0
	for i := 1; i <= len(stream); i++ {
		sameValue := false
		if i < len(stream) {
			pCur := powerVal(stream[i])
			pPrev := powerVal(stream[runStart])
			if pCur > 0 && pPrev > 0 && math.Abs(pCur-pPrev) < 0.5 {
				sameValue = true
			}
		}
		if !sameValue {
			val := powerVal(stream[runStart])
			if val > 0 && val > medianPower*thresholdMultiplier {
				durationS := stream[i-1].BETime().Sub(stream[runStart].BETime()).Seconds()
				if durationS >= minDurationS {
					regions = append(regions, StuckRegion{StartIdx: runStart, EndIdx: i - 1})
				}
			}
			runStart = i
		}
	}
	return regions
}

// powerVal extracts the power value from a BestEffortPoint. Returns 0 for nil.
func powerVal[T model.BestEffortPoint](p T) float64 {
	if pw := p.BEPowerW(); pw != nil {
		return float64(*pw)
	}
	return 0
}

// medianPowerFromStream computes the median non-zero power value across the
// stream. It should be called once per activity, not per category.
func medianPowerFromStream[T model.BestEffortPoint](stream []T) float64 {
	var vals []float64
	for _, p := range stream {
		if pw := p.BEPowerW(); pw != nil && *pw > 0 {
			vals = append(vals, float64(*pw))
		}
	}
	if len(vals) == 0 {
		return 0
	}
	slices.Sort(vals)
	mid := len(vals) / 2
	if len(vals)%2 == 0 {
		return (vals[mid-1] + vals[mid]) / 2
	}
	return vals[mid]
}

// isInStuckRegion returns true when the window [start, end] overlaps with any
// of the provided stuck regions.
func isInStuckRegion(start, end int, regions []StuckRegion) bool {
	for _, r := range regions {
		if start <= r.EndIdx && end >= r.StartIdx {
			return true
		}
	}
	return false
}

// gpsJumpThresholdM is the maximum plausible per-point distance delta in metres.
// Jumps larger than this indicate a GPS glitch and trigger a window reset.
const gpsJumpThresholdM = 200.0

// ComputeDistanceBestEffort finds the fastest elapsed time to cover cat.DistanceM
// using a two-pointer scan over the cumulative distance stream.
// GPS jumps >200 m between consecutive points cause the left pointer to advance
// past the jump so the window never straddles a glitch.
// Nil DistanceM points are skipped (not counted toward the window).
// Returns nil when the stream is shorter than the target distance.
func ComputeDistanceBestEffort[T model.BestEffortPoint](stream []T, cat Category) *EffortResult {
	target := cat.DistanceM
	n := len(stream)
	if n == 0 || target <= 0 {
		return nil
	}

	// Build index of valid points (non-nil DistanceM).
	type validPt struct {
		orig  int
		distM float64
	}
	valid := make([]validPt, 0, n)
	for i, p := range stream {
		if acc := p.BEGpsAccuracyM(); acc != nil && *acc > 50 {
			continue
		}
		if d := p.BEDistanceM(); d != nil {
			valid = append(valid, validPt{orig: i, distM: *d})
		}
	}
	if len(valid) < 2 {
		return nil
	}

	var bestElapsed int = math.MaxInt32
	var bestStart, bestEnd int

	left := 0
	for right := 1; right < len(valid); right++ {
		// GPS-jump detection: if the raw delta between consecutive valid points
		// exceeds the threshold, advance left past the jump point.
		delta := valid[right].distM - valid[right-1].distM
		if delta > gpsJumpThresholdM {
			left = right
			continue
		}

		// Shrink window from the left if it straddles a jump.
		for left < right {
			windowDist := valid[right].distM - valid[left].distM
			if windowDist >= target {
				elapsed := int(stream[valid[right].orig].BETime().Sub(stream[valid[left].orig].BETime()).Seconds())
				if elapsed < bestElapsed {
					bestElapsed = elapsed
					bestStart = valid[left].orig
					bestEnd = valid[right].orig
				}
				left++
			} else {
				break
			}
		}
	}

	if bestElapsed == math.MaxInt32 {
		return nil
	}
	return &EffortResult{
		Category:       cat.Key,
		ElapsedTimeS:   bestElapsed,
		ValueNumeric:   target,
		StreamStartIdx: bestStart,
		StreamEndIdx:   bestEnd,
	}
}

// ComputeElevationBestEffort finds the fastest elapsed time to accumulate
// cat.ElevationM of positive elevation gain using a two-pointer scan.
// liftIndices contains stream indices corresponding to ski-lift or similar
// assisted ascent segments; positive deltas at those indices are excluded.
// Returns nil when the stream never reaches the target gain.
func ComputeElevationBestEffort[T model.BestEffortPoint](stream []T, cat Category, liftIndices map[int]bool) *EffortResult {
	target := cat.ElevationM
	n := len(stream)
	if n == 0 || target <= 0 {
		return nil
	}

	// Precompute cumulative positive gain array (excluding lift indices).
	cumGain := make([]float64, n)
	cumGain[0] = 0
	for i := 1; i < n; i++ {
		prev := stream[i-1].BEAltitudeM()
		curr := stream[i].BEAltitudeM()
		var delta float64
		accI := stream[i].BEGpsAccuracyM()
		accPrev := stream[i-1].BEGpsAccuracyM()
		gpsOK := (accI == nil || *accI <= 50) && (accPrev == nil || *accPrev <= 50)
		if prev != nil && curr != nil && !liftIndices[i] && gpsOK {
			d := *curr - *prev
			if d > 0 {
				delta = d
			}
		}
		cumGain[i] = cumGain[i-1] + delta
	}

	var bestElapsed int = math.MaxInt32
	var bestStart, bestEnd int

	left := 0
	for right := 1; right < n; right++ {
		for left < right {
			windowGain := cumGain[right] - cumGain[left]
			if windowGain >= target {
				elapsed := int(stream[right].BETime().Sub(stream[left].BETime()).Seconds())
				if elapsed < bestElapsed {
					bestElapsed = elapsed
					bestStart = left
					bestEnd = right
				}
				left++
			} else {
				break
			}
		}
	}

	if bestElapsed == math.MaxInt32 {
		return nil
	}
	return &EffortResult{
		Category:       cat.Key,
		ElapsedTimeS:   bestElapsed,
		ValueNumeric:   target,
		StreamStartIdx: bestStart,
		StreamEndIdx:   bestEnd,
	}
}

// ComputePowerBestEffort finds the highest time-weighted mean power over a time
// window of exactly cat.DurationS seconds using a time-based sliding window.
//
// Nil PowerW values are treated as 0 W (coasting), consistent with the NP
// standard where stopped/freewheeling seconds still count.
//
// Each sample i is weighted by its duration: the interval from time[i] to
// time[i+1] (or 1 s for the last sample in the stream). This correctly handles
// Smart Recording where the device may record at 2–7 s intervals rather than 1 Hz.
// At 1 Hz the time-weighted mean equals the simple sample average.
//
// The window advances by time[right] - time[left] >= targetDuration.
// stuckRegions must be computed once per activity via findStuckPowerRegions;
// pass nil if no stuck-region detection is needed (e.g. when no power data).
// Returns nil when the stream is shorter than the target duration.
func ComputePowerBestEffort[T model.BestEffortPoint](stream []T, cat Category, stuckRegions []StuckRegion) *EffortResult {
	targetDuration := float64(cat.DurationS)
	n := len(stream)
	if n == 0 || targetDuration <= 0 {
		return nil
	}

	// Precompute per-sample duration (seconds to next sample; 1 s for last).
	dt := make([]float64, n)
	for i := 0; i < n-1; i++ {
		d := stream[i+1].BETime().Sub(stream[i].BETime()).Seconds()
		if d <= 0 {
			d = 1
		}
		dt[i] = d
	}
	dt[n-1] = 1

	// Precompute power values (nil → 0).
	power := make([]float64, n)
	for i, p := range stream {
		if pw := p.BEPowerW(); pw != nil {
			power[i] = float64(*pw)
		}
	}

	// Prefix sums for O(1) time-weighted mean queries over [left, right]:
	//   mean = (cumPowerTime[right+1] - cumPowerTime[left]) /
	//          (cumTime[right+1]      - cumTime[left])
	cumPowerTime := make([]float64, n+1)
	cumTime := make([]float64, n+1)
	for i := range n {
		cumPowerTime[i+1] = cumPowerTime[i] + power[i]*dt[i]
		cumTime[i+1] = cumTime[i] + dt[i]
	}

	var bestMean float64 = -1
	var bestStart, bestEnd int
	found := false

	left := 0
	for right := 1; right < n; right++ {
		windowSec := stream[right].BETime().Sub(stream[left].BETime()).Seconds()
		// Advance left to keep window at exactly the target duration.
		for left < right-1 {
			nextWindowSec := stream[right].BETime().Sub(stream[left+1].BETime()).Seconds()
			if nextWindowSec >= targetDuration {
				left++
			} else {
				break
			}
		}
		windowSec = stream[right].BETime().Sub(stream[left].BETime()).Seconds()
		if windowSec < targetDuration {
			continue
		}
		// Time-weighted mean power over [left, right] inclusive.
		totalTime := cumTime[right+1] - cumTime[left]
		if totalTime <= 0 {
			continue
		}
		mean := (cumPowerTime[right+1] - cumPowerTime[left]) / totalTime
		if !found || mean > bestMean {
			bestMean = mean
			bestStart = left
			bestEnd = right
			found = true
		}
	}

	if !found {
		return nil
	}
	return &EffortResult{
		Category:       cat.Key,
		ElapsedTimeS:   cat.DurationS,
		ValueNumeric:   bestMean,
		StreamStartIdx: bestStart,
		StreamEndIdx:   bestEnd,
		Suspicious:     isInStuckRegion(bestStart, bestEnd, stuckRegions),
	}
}

// ComputeSuperlative returns an EffortResult for a TypeValueSuperlative category
// by directly recording the provided summary value (e.g. total distance,
// total elevation gain, biggest climb, total duration).
// The caller is responsible for selecting the appropriate value for the category.
func ComputeSuperlative(cat Category, distanceM, elevGainM, biggestClimbM, durationS float64) *EffortResult {
	var value float64
	switch cat.Key {
	case "sup_longest_distance":
		value = distanceM
	case "sup_most_elevation":
		value = elevGainM
	case "sup_biggest_climb":
		value = biggestClimbM
	case "sup_longest_duration":
		value = durationS
	default:
		// Generic fallback: use distanceM as the primary value.
		value = distanceM
	}
	if value <= 0 {
		return nil
	}
	return &EffortResult{
		Category:       cat.Key,
		ElapsedTimeS:   int(durationS),
		ValueNumeric:   value,
		StreamStartIdx: 0,
		StreamEndIdx:   0,
	}
}
