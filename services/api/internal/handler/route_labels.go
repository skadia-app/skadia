package handler

import (
	"fmt"
	"slices"
)

// assignLabels assigns a VariantID, VariantLabelKey and dynamic Descriptors to
// each selected candidate. Input selectedIdx refers to indices into pool.
//
// Output is sorted by DistanceM ascending for UX consistency.
// K=1 → "only" with no Descriptors.
// K=2 → "shortest" / "longest".
// K=3 → "shortest" / "medium" / "longest".
// K≥4 → "shortest", "medium" (or numbered "medium-2", "medium-3" …), "longest".
func assignLabels(
	pool []RouteCandidate,
	selectedIdx []int,
	targetDistanceM float64,
	coverageAvailable bool,
) []LabeledRoute {
	if len(selectedIdx) == 0 {
		return nil
	}

	// Copy and sort indices by DistanceM ascending.
	sorted := make([]int, len(selectedIdx))
	copy(sorted, selectedIdx)
	slices.SortFunc(sorted, func(a, b int) int {
		da := pool[a].DistanceM
		db := pool[b].DistanceM
		switch {
		case da < db:
			return -1
		case da > db:
			return 1
		default:
			return 0
		}
	})

	k := len(sorted)

	// K=1: single route, no comparative descriptors.
	if k == 1 {
		return []LabeledRoute{
			{
				Candidate:       pool[sorted[0]],
				VariantID:       "only",
				VariantLabelKey: "only",
				Descriptors:     nil,
			},
		}
	}

	// Build label keys for positions.
	labelKeys := make([]string, k)
	labelKeys[0] = "shortest"
	labelKeys[k-1] = "longest"
	mediumCount := k - 2
	for i := 1; i < k-1; i++ {
		pos := i // 1-based middle position
		if mediumCount == 1 {
			labelKeys[i] = "medium"
		} else {
			labelKeys[i] = fmt.Sprintf("medium-%d", pos+1)
		}
	}

	// Build candidate slices for descriptor comparison (all candidates in pool
	// that were selected, excluding the one being described).
	result := make([]LabeledRoute, k)
	for i, idx := range sorted {
		me := pool[idx]

		// Collect others (the remaining selected candidates, excluding me).
		others := make([]RouteCandidate, 0, k-1)
		for j, otherIdx := range sorted {
			if j != i {
				others = append(others, pool[otherIdx])
			}
		}

		result[i] = LabeledRoute{
			Candidate:       me,
			VariantID:       labelKeys[i],
			VariantLabelKey: labelKeys[i],
			Descriptors: computeDescriptorsFor(
				me, others, targetDistanceM, coverageAvailable,
			),
		}
	}
	return result
}

// computeDescriptorsFor computes dynamic variant descriptors for a single
// candidate relative to a peer set (others). Returns nil when there is nothing
// distinctive to report.
//
// targetDistanceM is the user-requested target distance; it is used only for
// the target_distance descriptor.
//
// Coverage-based descriptors are gated on both coverageAvailable AND
// me.HasCoverage being true; this prevents phantom "greener" labels when the
// coverage layer has no data.
func computeDescriptorsFor(
	me RouteCandidate,
	others []RouteCandidate,
	targetDistanceM float64,
	coverageAvailable bool,
) []VariantDescriptor {
	if len(others) == 0 {
		return nil
	}

	// --- Pre-compute min/max across others ---
	var (
		maxOtherAscent    float64
		minOtherAscent    float64
		maxOtherForest    float32
		maxOtherRiver     float32
		maxOtherProtected float32
		minOtherTrafficNoise float32
		minOtherDistDelta float64
	)

	maxOtherAscent = others[0].AscentM
	minOtherAscent = others[0].AscentM
	maxOtherForest = others[0].MergedForestPct
	maxOtherRiver = others[0].MergedRiverProximity
	maxOtherProtected = others[0].MergedProtectedArea
	minOtherTrafficNoise = (others[0].MergedTrafficClass + others[0].MergedNoiseScore) / 2.0
	minOtherDistDelta = absDeltaFloat64(others[0].DistanceM, targetDistanceM)

	for _, o := range others[1:] {
		if o.AscentM > maxOtherAscent {
			maxOtherAscent = o.AscentM
		}
		if o.AscentM < minOtherAscent {
			minOtherAscent = o.AscentM
		}
		if o.MergedForestPct > maxOtherForest {
			maxOtherForest = o.MergedForestPct
		}
		if o.MergedRiverProximity > maxOtherRiver {
			maxOtherRiver = o.MergedRiverProximity
		}
		if o.MergedProtectedArea > maxOtherProtected {
			maxOtherProtected = o.MergedProtectedArea
		}
		combo := (o.MergedTrafficClass + o.MergedNoiseScore) / 2.0
		if combo < minOtherTrafficNoise {
			minOtherTrafficNoise = combo
		}
		delta := absDeltaFloat64(o.DistanceM, targetDistanceM)
		if delta < minOtherDistDelta {
			minOtherDistDelta = delta
		}
	}

	var out []VariantDescriptor

	// --- high_ascent ---
	ascentDelta := me.AscentM - maxOtherAscent
	if ascentDelta >= 30 && me.AscentM >= 1.2*maxOtherAscent {
		floor := maxOtherAscent
		if floor < 100 {
			floor = 100
		}
		out = append(out, VariantDescriptor{
			Key:          "high_ascent",
			Magnitude:    ascentDelta / floor,
			DisplayValue: fmt.Sprintf("+%dm", int(ascentDelta)),
		})
	}

	// --- low_ascent ---
	ascentDeficit := minOtherAscent - me.AscentM
	if ascentDeficit >= 30 && me.AscentM <= 0.8*minOtherAscent {
		floor := minOtherAscent
		if floor < 100 {
			floor = 100
		}
		out = append(out, VariantDescriptor{
			Key:          "low_ascent",
			Magnitude:    ascentDeficit / floor,
			DisplayValue: fmt.Sprintf("-%dm", int(ascentDeficit)),
		})
	}

	// Coverage-gated descriptors: only when coverageAvailable AND me.HasCoverage.
	if coverageAvailable && me.HasCoverage {
		// --- high_forest ---
		myForest := me.MergedForestPct
		if myForest > 1.0 {
			myForest = 1.0 // defensive clamp for scale-bug inheritance
		}
		forestDelta := myForest - maxOtherForest
		if myForest > 0 && forestDelta >= 0.10 {
			out = append(out, VariantDescriptor{
				Key:          "high_forest",
				Magnitude:    float64(forestDelta),
				DisplayValue: fmt.Sprintf("%.0f%%", myForest*100),
			})
		}

		// --- high_water ---
		riverDelta := me.MergedRiverProximity - maxOtherRiver
		if me.MergedRiverProximity > 0 && riverDelta >= 0.15 {
			out = append(out, VariantDescriptor{
				Key:          "high_water",
				Magnitude:    float64(riverDelta),
				DisplayValue: fmt.Sprintf("%.0f%%", me.MergedRiverProximity*100),
			})
		}

		// --- high_protected ---
		protectedDelta := me.MergedProtectedArea - maxOtherProtected
		if me.MergedProtectedArea > 0 && protectedDelta >= 0.33 {
			out = append(out, VariantDescriptor{
				Key:          "high_protected",
				Magnitude:    float64(protectedDelta),
				DisplayValue: fmt.Sprintf("%.0f%%", me.MergedProtectedArea*100),
			})
		}

		// --- low_traffic_noise ---
		myCombo := (me.MergedTrafficClass + me.MergedNoiseScore) / 2.0
		comboDeficit := minOtherTrafficNoise - myCombo
		if comboDeficit >= 0.15 {
			out = append(out, VariantDescriptor{
				Key:          "low_traffic_noise",
				Magnitude:    float64(comboDeficit),
				DisplayValue: fmt.Sprintf("%.0f%%", myCombo*100),
			})
		}
	}

	// --- target_distance ---
	if targetDistanceM > 0 {
		meDelta := absDeltaFloat64(me.DistanceM, targetDistanceM)
		withinBand := meDelta <= 0.05*targetDistanceM
		// me must be clearly closer than the nearest other candidate.
		clearlyBetter := minOtherDistDelta > 2*meDelta
		if withinBand && clearlyBetter {
			out = append(out, VariantDescriptor{
				Key:          "target_distance",
				Magnitude:    meDelta / targetDistanceM,
				DisplayValue: fmt.Sprintf("%.1fkm", me.DistanceM/1000.0),
			})
		}
	}

	return out
}

// absDeltaFloat64 returns |a - b|.
func absDeltaFloat64(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}
