package routing

// CandidateSelection identifies a reachable node + which of its Pareto states
// to use for variant construction.
type CandidateSelection struct {
	NodeIdx  int // Index in reachable slice
	StateIdx int // Index in reachable[NodeIdx].States
}

// CandidateMetrics bundles route-level metrics for diversity selection.
// Coverage dimensions use pointers to distinguish "no coverage" from "zero coverage".
type CandidateMetrics struct {
	DistanceM  float64
	AscentM    float64
	AvgFlat    float32
	GreenScore *float32 // nil = keine Coverage-Daten für diese Route
	CalmScore  *float32 // nil = keine Coverage-Daten für diese Route
	NodeID     uint64   // deterministic tiebreaker
}
