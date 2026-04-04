package bestefforts

import (
	"math"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// EffortResult holds the computed best effort for a single category.
type EffortResult struct {
	Category       string
	ElapsedTimeS   int
	ValueNumeric   float64
	StreamStartIdx int
	StreamEndIdx   int
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
func ComputeDistanceBestEffort(stream []model.StreamPoint, cat Category) *EffortResult {
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
		if p.DistanceM != nil {
			valid = append(valid, validPt{orig: i, distM: *p.DistanceM})
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
				elapsed := int(stream[valid[right].orig].Time.Sub(stream[valid[left].orig].Time).Seconds())
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
func ComputeElevationBestEffort(stream []model.StreamPoint, cat Category, liftIndices map[int]bool) *EffortResult {
	target := cat.ElevationM
	n := len(stream)
	if n == 0 || target <= 0 {
		return nil
	}

	// Precompute cumulative positive gain array (excluding lift indices).
	cumGain := make([]float64, n)
	cumGain[0] = 0
	for i := 1; i < n; i++ {
		prev := stream[i-1].AltitudeM
		curr := stream[i].AltitudeM
		var delta float64
		if prev != nil && curr != nil && !liftIndices[i] {
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
				elapsed := int(stream[right].Time.Sub(stream[left].Time).Seconds())
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

// ComputePowerBestEffort finds the highest mean power over a time window of
// exactly cat.DurationS seconds using a time-based sliding window.
// Nil PowerW values are treated as 0 W (coasting), consistent with the NP
// standard where stopped/freewheeling seconds still count.
// The window advances by time[right] - time[left] >= targetDuration.
// Returns nil when the stream is shorter than the target duration.
func ComputePowerBestEffort(stream []model.StreamPoint, cat Category) *EffortResult {
	targetDuration := float64(cat.DurationS)
	n := len(stream)
	if n == 0 || targetDuration <= 0 {
		return nil
	}

	// Precompute power values (nil → 0).
	power := make([]float64, n)
	for i, p := range stream {
		if p.PowerW != nil {
			power[i] = float64(*p.PowerW)
		}
	}

	// Precompute running sum for O(1) mean queries.
	cumPower := make([]float64, n+1)
	for i := 0; i < n; i++ {
		cumPower[i+1] = cumPower[i] + power[i]
	}

	var bestMean float64 = -1
	var bestStart, bestEnd int
	found := false

	left := 0
	for right := 1; right < n; right++ {
		windowSec := stream[right].Time.Sub(stream[left].Time).Seconds()
		// Advance left to keep window at exactly the target duration.
		for left < right-1 {
			nextWindowSec := stream[right].Time.Sub(stream[left+1].Time).Seconds()
			if nextWindowSec >= targetDuration {
				left++
			} else {
				break
			}
		}
		windowSec = stream[right].Time.Sub(stream[left].Time).Seconds()
		if windowSec < targetDuration {
			continue
		}
		// Mean power over [left, right] inclusive.
		count := right - left + 1
		mean := (cumPower[right+1] - cumPower[left]) / float64(count)
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
