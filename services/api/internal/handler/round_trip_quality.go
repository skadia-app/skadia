package handler

import "github.com/skadia-app/skadia/services/api/internal/routing"

// LoopQuality classifies a round-trip suggestion by how "loop-like" it is.
type LoopQuality string

const (
	LoopQualityLoop        LoopQuality = "loop"
	LoopQualityPartialLoop LoopQuality = "partial_loop"
	LoopQualityOutAndBack  LoopQuality = "out_and_back"
)

// RoundTripOptions configures the quality-gate thresholds.
type RoundTripOptions struct {
	MaxAvgFlatDifference  float32
	MaxAvgFlatVsBaseline  float32
	MaxLeg2DistanceFactor float32
	OverlapCheckThreshold float64
	MaxOverlapThreshold   float64
	SoftPenaltyFactor     float32
}

// DefaultRoundTripOptions returns the tested defaults.
func DefaultRoundTripOptions() RoundTripOptions {
	return RoundTripOptions{
		MaxAvgFlatDifference:  0.5,
		MaxAvgFlatVsBaseline:  1.5,
		MaxLeg2DistanceFactor: 1.8,
		OverlapCheckThreshold: 0.60,
		MaxOverlapThreshold:   0.95,
		SoftPenaltyFactor:     1.3,
	}
}

// RoundTripResult is the outcome of one variant's round-trip construction.
type RoundTripResult struct {
	Leg1        *routing.RouteResult
	Leg2        *routing.RouteResult
	LoopQuality LoopQuality
	OverlapPct  float64
	AvgFlatLeg1 float32
	AvgFlatLeg2 float32
}

// passesQualityGate returns true iff leg2 is not significantly worse than leg1.
func passesQualityGate(
	leg1, leg2 *routing.RouteResult,
	baselineAvgFlat float32,
	opts RoundTripOptions,
) bool {
	qualityOK := false
	diff := leg2.AvgFlat() - leg1.AvgFlat()
	if diff <= opts.MaxAvgFlatDifference {
		qualityOK = true
	} else if leg2.AvgFlat() <= opts.MaxAvgFlatVsBaseline*baselineAvgFlat {
		qualityOK = true
	}
	if !qualityOK {
		return false
	}

	if leg1.DistanceM == 0 {
		return true
	}
	if leg2.DistanceM > float64(opts.MaxLeg2DistanceFactor)*leg1.DistanceM {
		return false
	}

	return true
}
