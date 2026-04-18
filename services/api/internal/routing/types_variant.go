package routing

// CandidateSelection identifies a reachable node + which of its Pareto states
// to use for variant construction.
//
// Consumed by BuildCandidatePool and PickDiverseIndices in the V2
// route-suggestion pipeline (routing.route_variants_v2 feature flag).
type CandidateSelection struct {
	NodeIdx  int // Index in reachable slice
	StateIdx int // Index in reachable[NodeIdx].States
}

// CandidateMetrics bundles route-level metrics for diversity selection.
// Coverage dimensions use pointers to distinguish "no coverage" from "zero coverage".
//
// Consumed by BuildCandidatePool and PickDiverseIndices in the V2
// route-suggestion pipeline (routing.route_variants_v2 feature flag).
type CandidateMetrics struct {
	DistanceM  float64
	AscentM    float64
	AvgFlat    float32
	GreenScore *float32 // nil = no coverage data for this route
	CalmScore  *float32 // nil = no coverage data for this route
	NodeID     uint64   // deterministic tiebreaker
}
