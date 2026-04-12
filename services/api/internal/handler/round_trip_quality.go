package handler

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/routing"
)

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
	SoftPenaltyFactor     float32
}

// DefaultRoundTripOptions returns the tested defaults.
func DefaultRoundTripOptions() RoundTripOptions {
	return RoundTripOptions{
		MaxAvgFlatDifference:  0.5,
		MaxAvgFlatVsBaseline:  1.5,
		MaxLeg2DistanceFactor: 1.8,
		OverlapCheckThreshold: 0.60,
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

// suggestRoundTripVariant constructs a round-trip from a Pareto-selected variant
// node back to the source. It:
//  1. Reconstructs leg1 from the Pareto slab (source → variant node).
//  2. Routes leg2 naturally (variant node → source).
//  3. Checks WayID overlap and quality gate.
//  4. If overlap is high, retries leg2 with soft penalties on leg1's way IDs.
//  5. Classifies the result as loop / partial_loop / out_and_back.
func suggestRoundTripVariant(
	ctx context.Context,
	router *routing.UnifiedRouter,
	rs *routing.ReachableState,
	srcLat, srcLon float64,
	selected routing.SelectedVariant,
	cfg *routing.CostConfig,
	opts RoundTripOptions,
	baselineAvgFlat float32,
) (*RoundTripResult, error) {
	if opts.SoftPenaltyFactor < 1.0 {
		return nil, fmt.Errorf("SoftPenaltyFactor must be >= 1.0, got %f", opts.SoftPenaltyFactor)
	}

	// --- Leg 1: reconstruct from Pareto state ---
	paretoNode := selected.Node
	stateSlabID := paretoNode.StateSlabIDs[selected.StateIdx]
	leg1, leg1Edges, err := routing.ReconstructAndBuildRoute(rs, stateSlabID, router.Reader())
	if err != nil {
		return nil, fmt.Errorf("leg1 reconstruction: %w", err)
	}
	leg1WayIDs := routing.ExtractWayIDsFromEdges(leg1Edges, router.Reader())

	// --- Leg 2: natural route back ---
	leg2Natural, err := router.RouteVia(ctx,
		[]routing.LatLon{
			{Lat: paretoNode.Lat, Lon: paretoNode.Lon},
			{Lat: srcLat, Lon: srcLon},
		}, cfg)
	if err != nil {
		return nil, fmt.Errorf("leg2 natural: %w", err)
	}

	// ExtractWayIDsFromRouteResult reads Segments[].OsmWayID. RouteVia builds segments via
	// makeSegmentFromTags which always sets OsmWayID=0 (not available through GraphReader).
	// As a result leg2NaturalWayIDs is always empty and overlap is always 0 — meaning the
	// high-overlap branch (penalty rerun) is never triggered via this path. The overlap
	// metric is therefore unreliable for RouteVia results. leg1WayIDs (from ExtractWayIDsFromEdges
	// via ReconstructAndBuildRoute) is correct because it uses reader.EdgeOSMID() directly.
	// TODO: populate Segments[].OsmWayID in retrackUnified to fix overlap detection for leg2.
	leg2NaturalWayIDs := routing.ExtractWayIDsFromRouteResult(leg2Natural)
	overlap := routing.ComputeWayIDOverlap(leg1WayIDs, leg2NaturalWayIDs)
	naturalQualityOK := passesQualityGate(leg1, leg2Natural, baselineAvgFlat, opts)

	// --- Decision tree ---
	var leg2Final *routing.RouteResult
	var quality LoopQuality

	switch {
	case overlap < opts.OverlapCheckThreshold && naturalQualityOK:
		leg2Final = leg2Natural
		quality = LoopQualityLoop

	case overlap < opts.OverlapCheckThreshold && !naturalQualityOK:
		leg2Final = leg2Natural
		quality = LoopQualityPartialLoop // was: LoopQualityOutAndBack — low overlap means legs differ, so it IS a loop, just poor quality

	default:
		// High overlap — try penalty rerun
		penaltyCfg := *cfg
		penaltyCfg.WayPenalties = make(map[int64]float32, len(leg1WayIDs))
		for wid := range leg1WayIDs {
			penaltyCfg.WayPenalties[wid] = opts.SoftPenaltyFactor
		}

		leg2Penalized, penErr := router.RouteVia(ctx,
			[]routing.LatLon{
				{Lat: paretoNode.Lat, Lon: paretoNode.Lon},
				{Lat: srcLat, Lon: srcLon},
			}, &penaltyCfg)

		if penErr != nil || leg2Penalized == nil {
			leg2Final = leg2Natural
			if naturalQualityOK {
				quality = LoopQualityPartialLoop
			} else {
				quality = LoopQualityOutAndBack
			}
		} else if passesQualityGate(leg1, leg2Penalized, baselineAvgFlat, opts) {
			leg2Final = leg2Penalized
			newOverlap := routing.ComputeWayIDOverlap(leg1WayIDs,
				routing.ExtractWayIDsFromRouteResult(leg2Penalized))
			if newOverlap < opts.OverlapCheckThreshold {
				quality = LoopQualityLoop
			} else {
				quality = LoopQualityPartialLoop
			}
		} else {
			leg2Final = leg2Natural
			if naturalQualityOK {
				quality = LoopQualityPartialLoop
			} else {
				quality = LoopQualityOutAndBack
			}
		}
	}

	routing.PenaltyRerunCount.WithLabelValues(string(quality)).Inc()

	return &RoundTripResult{
		Leg1:        leg1,
		Leg2:        leg2Final,
		LoopQuality: quality,
		OverlapPct:  overlap,
		AvgFlatLeg1: leg1.AvgFlat(),
		AvgFlatLeg2: leg2Final.AvgFlat(),
	}, nil
}
