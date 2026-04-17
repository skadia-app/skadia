package routing

// CandidateSelection identifies a reachable node + which of its Pareto states
// to use for variant construction.
//
// TODO(route-variants-v2): consumed by BuildCandidatePool (Phase 2) and
// PickDiverseIndices (Phase 3). Not yet wired into the handler response path.
type CandidateSelection struct {
	NodeIdx  int // Index in reachable slice
	StateIdx int // Index in reachable[NodeIdx].States
}

// CandidateMetrics bundles route-level metrics for diversity selection.
// Coverage dimensions use pointers to distinguish "no coverage" from "zero coverage".
//
// TODO(route-variants-v2): produced by handler pipeline (Phase 4), consumed by
// computeZScores + PickDiverseIndices (Phase 3). Not yet wired.
type CandidateMetrics struct {
	DistanceM  float64
	AscentM    float64
	AvgFlat    float32
	GreenScore *float32 // nil = no coverage data for this route
	CalmScore  *float32 // nil = no coverage data for this route
	NodeID     uint64   // deterministic tiebreaker
}
