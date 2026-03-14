package routing

// SparseState is the map-based working state for overflow queries.
// STUB — full implementation in Task 7 (sparse.go).
type SparseState struct {
	bestCost   map[uint32]float32
	parentNode map[uint32]uint32
	parentEdge map[uint32]uint32
	frontier   Frontier
}
