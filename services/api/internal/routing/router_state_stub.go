package routing

// RouterState holds pre-allocated working arrays for a single routing query.
// STUB — full implementation in Task 7 (router.go).
type RouterState struct {
	bestCost   []float32
	parentNode []uint32
	parentEdge []uint32
	nodeGen    []uint32
	generation uint32
	frontier   Frontier
}
