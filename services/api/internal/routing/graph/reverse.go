package graph

// BuildReverseAdj constructs the reverse adjacency lists from the forward CSR.
// O(E) time and O(E) space. Call after deserialization.
func (g *Graph) BuildReverseAdj() {
	if g.NodeCount == 0 || g.EdgeCount == 0 {
		return
	}

	// Count incoming edges per node
	inDeg := make([]uint32, g.NodeCount)
	for ei := range g.EdgeCount {
		inDeg[g.Targets[ei]]++
	}

	// Build reverse offsets (prefix sum)
	g.RevOffsets = make([]uint32, g.NodeCount+1)
	for i := range g.NodeCount {
		g.RevOffsets[i+1] = g.RevOffsets[i] + inDeg[i]
	}

	// Fill reverse targets and edge indices
	g.RevTargets = make([]uint32, g.EdgeCount)
	g.RevEdgeIdx = make([]uint32, g.EdgeCount)
	pos := make([]uint32, g.NodeCount) // current insert position per node
	for src := range g.NodeCount {
		for ei := g.Offsets[src]; ei < g.Offsets[src+1]; ei++ {
			tgt := g.Targets[ei]
			idx := g.RevOffsets[tgt] + pos[tgt]
			g.RevTargets[idx] = src
			g.RevEdgeIdx[idx] = ei
			pos[tgt]++
		}
	}
}
