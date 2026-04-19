package routing

import "math"

// CandidateSelection identifies a reachable node + which of its Pareto states
// to use for variant construction.
//
// Consumed by BuildCandidatePool and PickDiverseIndices in the
// route-suggestion pipeline.
type CandidateSelection struct {
	NodeIdx  int // Index in reachable slice
	StateIdx int // Index in reachable[NodeIdx].States
}

// CandidateMetrics bundles route-level metrics for diversity selection.
// Coverage dimensions use pointers to distinguish "no coverage" from "zero coverage".
//
// Consumed by BuildCandidatePool and PickDiverseIndices in the
// route-suggestion pipeline.
type CandidateMetrics struct {
	DistanceM  float64
	AscentM    float64
	AvgFlat    float32
	GreenScore *float32 // nil = no coverage data for this route
	CalmScore  *float32 // nil = no coverage data for this route
	NodeID     uint64   // deterministic tiebreaker
}

// MinVariantDistanceM returns the minimum spatial separation (in metres)
// that two selected variant nodes must have. The floor of 300 m prevents
// trivial near-duplicate variants on short target distances.
func MinVariantDistanceM(targetDistanceM float64) float64 {
	return math.Max(300.0, 0.05*targetDistanceM)
}
