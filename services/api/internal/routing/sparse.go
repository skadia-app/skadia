// sparse.go
package routing

// SparseState is the map-based working state for overflow queries.
// Used when the hot-pool (flat arrays) is exhausted.
// Memory: ~100-300 MB per query at 500K-1M visited nodes.
type SparseState struct {
	bestCost   map[uint32]float32
	parentNode map[uint32]uint32
	parentEdge map[uint32]uint32

	// Per-node state maps for hysteresis and last-edge direction.
	// Populated during A* expansion; read when popping (replaces FrontierEntry fields
	// removed in Task 8). Task 11 (unified A*) will replace this with the NodeState struct.
	nodeHyState  map[uint32]PathState
	nodeLastEdge map[uint32]crossLastEdge

	frontier Frontier
}

func newSparseState() *SparseState {
	return &SparseState{
		bestCost:     make(map[uint32]float32, 500_000),
		parentNode:   make(map[uint32]uint32, 500_000),
		parentEdge:   make(map[uint32]uint32, 500_000),
		nodeHyState:  make(map[uint32]PathState, 500_000),
		nodeLastEdge: make(map[uint32]crossLastEdge, 500_000),
	}
}

// clearMaps resets sparse maps for pass reuse.
func (s *SparseState) clearMaps() {
	clear(s.bestCost)
	clear(s.parentNode)
	clear(s.parentEdge)
	clear(s.nodeHyState)
	clear(s.nodeLastEdge)
}
