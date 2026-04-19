package handler

import (
	"fmt"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// outdoorThresholds bundles fire-trigger + coverage parameters for F26 descriptors.
// Defaults at defaultOutdoorThresholds. profileOutdoorThresholds reserved for future eMTB.
type outdoorThresholds struct {
	MtbTechnicalMin  uint8
	AlpineHikingMin  uint8
	LowVisMaxTrigger uint8
	LowVisIntPct     float32
	LowVisIntDelta   float32
	CoverageGatePct  float32
}

var defaultOutdoorThresholds = outdoorThresholds{
	MtbTechnicalMin:  3,    // S2+ encoded
	AlpineHikingMin:  4,    // T4+ encoded
	LowVisMaxTrigger: 4,    // bad+
	LowVisIntPct:     0.20,
	LowVisIntDelta:   0.10,
	CoverageGatePct:  0.20,
}

var profileOutdoorThresholds = map[string]outdoorThresholds{
	// empty — reserved for future eMTB etc.
}

func thresholdsFor(profile string) outdoorThresholds {
	if t, ok := profileOutdoorThresholds[profile]; ok {
		return t
	}
	return defaultOutdoorThresholds
}

// Threshold seeds for variant descriptors.
// These were validated via POC (Phase 0) and may be re-tuned once
// real coverage data from the next tile rebuild is available.
const (
	// Ascent descriptors
	ascentDeltaThresholdM = 30.0  // min absolute ascent delta (meters) to trigger high/low_ascent
	ascentHighRatioGate   = 1.2   // me.AscentM / max(others.AscentM) must be ≥ this for high_ascent
	ascentLowRatioGate    = 0.8   // me.AscentM / min(others.AscentM) must be ≤ this for low_ascent
	ascentMagnitudeFloorM = 100.0 // divisor floor for ascent magnitude ratio (avoids /0)

	// Coverage descriptors (all absolute pp deltas)
	forestPpDelta    = 0.10
	waterPpDelta     = 0.15
	protectedPpDelta = 0.33
	trafficNoiseDelta = 0.15

	// Target-distance descriptor
	targetDistanceWithinFraction = 0.05 // |me - target| ≤ this × target
	targetDistanceBestRatio      = 2.0  // min_other_delta must be > this × me_delta
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
	profile string, // NEW
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

	thresholds := thresholdsFor(profile)

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
				me, others, targetDistanceM, coverageAvailable, thresholds,
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
	thresholds outdoorThresholds,
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
	if ascentDelta >= ascentDeltaThresholdM && me.AscentM >= ascentHighRatioGate*maxOtherAscent {
		floor := maxOtherAscent
		if floor < ascentMagnitudeFloorM {
			floor = ascentMagnitudeFloorM
		}
		out = append(out, VariantDescriptor{
			Key:          "high_ascent",
			Magnitude:    ascentDelta / floor,
			DisplayValue: fmt.Sprintf("+%dm", int(ascentDelta)),
		})
	}

	// --- low_ascent ---
	ascentDeficit := minOtherAscent - me.AscentM
	if ascentDeficit >= ascentDeltaThresholdM && me.AscentM <= ascentLowRatioGate*minOtherAscent {
		floor := minOtherAscent
		if floor < ascentMagnitudeFloorM {
			floor = ascentMagnitudeFloorM
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
		if myForest > 0 && forestDelta >= forestPpDelta {
			out = append(out, VariantDescriptor{
				Key:          "high_forest",
				Magnitude:    float64(forestDelta),
				DisplayValue: fmt.Sprintf("%.0f%%", myForest*100),
			})
		}

		// --- high_water ---
		riverDelta := me.MergedRiverProximity - maxOtherRiver
		if me.MergedRiverProximity > 0 && riverDelta >= waterPpDelta {
			out = append(out, VariantDescriptor{
				Key:          "high_water",
				Magnitude:    float64(riverDelta),
				DisplayValue: fmt.Sprintf("%.0f%%", me.MergedRiverProximity*100),
			})
		}

		// --- high_protected ---
		protectedDelta := me.MergedProtectedArea - maxOtherProtected
		if me.MergedProtectedArea > 0 && protectedDelta >= protectedPpDelta {
			out = append(out, VariantDescriptor{
				Key:          "high_protected",
				Magnitude:    float64(protectedDelta),
				DisplayValue: fmt.Sprintf("%.0f%%", me.MergedProtectedArea*100),
			})
		}

		// --- low_traffic_noise ---
		myCombo := (me.MergedTrafficClass + me.MergedNoiseScore) / 2.0
		comboDeficit := minOtherTrafficNoise - myCombo
		if comboDeficit >= trafficNoiseDelta {
			out = append(out, VariantDescriptor{
				Key:          "low_traffic_noise",
				Magnitude:    float64(comboDeficit),
				DisplayValue: fmt.Sprintf("%.0f%%", myCombo*100),
			})
		}
	}

	// --- F26 outdoor descriptors ---

	// mtb_technical
	if me.TaggedPctMtb >= thresholds.CoverageGatePct && me.MaxMtbScale >= thresholds.MtbTechnicalMin {
		var maxOther uint8
		for _, o := range others {
			if o.MaxMtbScale > maxOther {
				maxOther = o.MaxMtbScale
			}
		}
		if me.MaxMtbScale > maxOther {
			out = append(out, VariantDescriptor{
				Key:          "mtb_technical",
				Magnitude:    float64(me.MaxMtbScale),
				DisplayValue: "bis " + graph.MtbScaleShort(me.MaxMtbScale),
			})
		}
	} else if me.MaxMtbScale >= thresholds.MtbTechnicalMin {
		routingMetricDescriptorCoverageGated.WithLabelValues("mtb_technical").Inc()
	}

	// alpine_hiking
	if me.TaggedPctSac >= thresholds.CoverageGatePct && me.MaxSacScale >= thresholds.AlpineHikingMin {
		var maxOther uint8
		for _, o := range others {
			if o.MaxSacScale > maxOther {
				maxOther = o.MaxSacScale
			}
		}
		if me.MaxSacScale > maxOther {
			out = append(out, VariantDescriptor{
				Key:          "alpine_hiking",
				Magnitude:    float64(me.MaxSacScale),
				DisplayValue: "bis " + graph.SacScaleShort(me.MaxSacScale),
			})
		}
	} else if me.MaxSacScale >= thresholds.AlpineHikingMin {
		routingMetricDescriptorCoverageGated.WithLabelValues("alpine_hiking").Inc()
	}

	// exposed — binary, 1% coverage floor
	const exposedCoverageFloor float32 = 0.01
	hasMeVia := me.HasViaFerrataHighway || me.MaxViaFerrataScale > 0
	if hasMeVia && me.TaggedPctVia >= exposedCoverageFloor {
		hasOther := false
		for _, o := range others {
			if o.HasViaFerrataHighway || o.MaxViaFerrataScale > 0 {
				hasOther = true
				break
			}
		}
		if !hasOther {
			var display string
			if me.MaxViaFerrataScale > 0 {
				display = "bis " + graph.ViaFerrataScaleShort(me.MaxViaFerrataScale)
			} else {
				display = "Klettersteig-Abschnitt"
			}
			out = append(out, VariantDescriptor{
				Key:          "exposed",
				Magnitude:    float64(me.MaxViaFerrataScale),
				DisplayValue: display,
			})
		}
	} else if hasMeVia {
		routingMetricDescriptorCoverageGated.WithLabelValues("exposed").Inc()
	}

	// low_visibility — two-stage trigger
	if me.TaggedPctTv >= thresholds.CoverageGatePct {
		maxTrigger := me.MaxTrailVisibility >= thresholds.LowVisMaxTrigger
		intTrigger := false
		if !maxTrigger && me.IntermediatePctTv >= thresholds.LowVisIntPct {
			var maxOtherInt float32
			for _, o := range others {
				if o.IntermediatePctTv > maxOtherInt {
					maxOtherInt = o.IntermediatePctTv
				}
			}
			if me.IntermediatePctTv-maxOtherInt >= thresholds.LowVisIntDelta {
				intTrigger = true
			}
		}
		if maxTrigger || intTrigger {
			var display string
			if me.MaxTrailVisibility >= 4 {
				display = "bis " + tvDisplayName(me.MaxTrailVisibility)
			} else {
				display = fmt.Sprintf("%.0f%% mäßig sichtbar", me.IntermediatePctTv*100)
			}
			out = append(out, VariantDescriptor{
				Key:          "low_visibility",
				Magnitude:    float64(me.MaxTrailVisibility),
				DisplayValue: display,
			})
		}
	} else {
		wouldFireMax := me.MaxTrailVisibility >= thresholds.LowVisMaxTrigger
		wouldFireInt := me.IntermediatePctTv >= thresholds.LowVisIntPct
		if wouldFireMax || wouldFireInt {
			routingMetricDescriptorCoverageGated.WithLabelValues("low_visibility").Inc()
		}
	}

	// --- target_distance ---
	// target_distance: only meaningful when comparing to ≥ 2 other routes
	if targetDistanceM > 0 && len(others) >= 2 {
		meDelta := absDeltaFloat64(me.DistanceM, targetDistanceM)
		withinBand := meDelta <= targetDistanceWithinFraction*targetDistanceM
		// me must be clearly closer than the nearest other candidate.
		clearlyBetter := minOtherDistDelta > targetDistanceBestRatio*meDelta
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

// --- F26 German labels for trail_visibility display (user-facing) ---

var tvDisplayNames = [7]string{
	"",
	"sehr gut sichtbar",
	"gut sichtbar",
	"mäßig sichtbar",
	"schlecht sichtbar",
	"kaum sichtbar",
	"nicht sichtbar",
}

func tvDisplayName(v uint8) string {
	if int(v) >= len(tvDisplayNames) {
		return ""
	}
	return tvDisplayNames[v]
}
