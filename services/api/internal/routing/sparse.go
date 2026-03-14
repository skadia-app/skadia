// sparse.go
package routing

// SparseState is the map-based working state for overflow queries.
// Used when the hot-pool (flat arrays) is exhausted.
// Memory: ~100-300 MB per query at 500K-1M visited nodes.
type SparseState struct {
	bestCost   map[uint32]float32
	parentNode map[uint32]uint32
	parentEdge map[uint32]uint32

	frontier Frontier
}

func newSparseState() *SparseState {
	return &SparseState{
		bestCost:   make(map[uint32]float32, 500_000),
		parentNode: make(map[uint32]uint32, 500_000),
		parentEdge: make(map[uint32]uint32, 500_000),
	}
}

// clearMaps resets sparse maps for pass reuse.
func (s *SparseState) clearMaps() {
	clear(s.bestCost)
	clear(s.parentNode)
	clear(s.parentEdge)
}
