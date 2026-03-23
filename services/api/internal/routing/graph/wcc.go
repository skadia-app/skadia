package graph

// PruneWCC removes weakly connected components smaller than minSize.
// Uses union-find on the undirected version of the graph.
func PruneWCC(g *Graph, minSize int) *Graph {
	n := int(g.NodeCount)
	uf := newUnionFind(n)

	for i := uint32(0); i < g.NodeCount; i++ {
		for _, j := range g.Edges(i) {
			uf.union(int(i), int(j))
		}
	}

	// Find the largest component size.
	maxSize := 0
	for i := range n {
		root := uf.find(i)
		if uf.size[root] > maxSize {
			maxSize = uf.size[root]
		}
	}

	keep := make([]bool, n)
	for i := range n {
		root := uf.find(i)
		// Keep a node if its component is >= minSize, OR if it belongs to
		// the largest component (always keep the main graph body).
		if uf.size[root] >= minSize || uf.size[root] == maxSize {
			keep[i] = true
		}
	}

	keptCount := 0
	for _, k := range keep {
		if k {
			keptCount++
		}
	}
	if keptCount == n {
		return g
	}

	remap := make([]uint32, n)
	newID := uint32(0)
	for i := range n {
		if keep[i] {
			remap[i] = newID
			newID++
		}
	}

	newG := &Graph{
		NodeCount: uint32(keptCount),
		Version:   g.Version,
		BuildTime: g.BuildTime,
		BBox:      g.BBox,
	}

	newG.NodeLat = make([]int32, keptCount)
	newG.NodeLon = make([]int32, keptCount)
	newG.NodeEle = make([]int32, keptCount)
	newG.NodeTags = make([]uint8, keptCount)
	for i := range n {
		if keep[i] {
			j := remap[i]
			newG.NodeLat[j] = g.NodeLat[i]
			newG.NodeLon[j] = g.NodeLon[i]
			newG.NodeEle[j] = g.NodeEle[i]
			newG.NodeTags[j] = g.NodeTags[i]
		}
	}

	var newEdges []uint32
	var newDist []uint32
	var newTags1, newTags2 []uint64
	var newOSMIdx []uint32
	var keptOldPositions []uint32 // original edge indices for geometry/elevation lookup
	newOffsets := make([]uint32, keptCount+1)

	for i := uint32(0); i < g.NodeCount; i++ {
		if !keep[int(i)] {
			continue
		}
		newNode := remap[i]
		start := g.Offsets[i]
		end := g.Offsets[i+1]
		for pos := start; pos < end; pos++ {
			target := g.Targets[pos]
			if !keep[int(target)] {
				continue
			}
			newEdges = append(newEdges, remap[target])
			newDist = append(newDist, g.EdgeDist[pos])
			newTags1 = append(newTags1, g.EdgeTags1[pos])
			newTags2 = append(newTags2, g.EdgeTags2[pos])
			newOSMIdx = append(newOSMIdx, g.EdgeOSMIdx[pos])
			keptOldPositions = append(keptOldPositions, pos)
		}
		newOffsets[newNode+1] = uint32(len(newEdges))
	}

	// Rebuild geometry offsets (EdgeGeomOff/Lat/Lon)
	var newGeomLat, newGeomLon []int32
	newGeomOff := make([]uint32, len(newEdges)+1)
	if len(g.EdgeGeomOff) > 0 {
		for newIdx, oldIdx := range keptOldPositions {
			newGeomOff[newIdx] = uint32(len(newGeomLat))
			gStart := g.EdgeGeomOff[oldIdx]
			gEnd := g.EdgeGeomOff[oldIdx+1]
			if gEnd > gStart {
				newGeomLat = append(newGeomLat, g.EdgeGeomLat[gStart:gEnd]...)
				newGeomLon = append(newGeomLon, g.EdgeGeomLon[gStart:gEnd]...)
			}
		}
	}
	newGeomOff[len(newEdges)] = uint32(len(newGeomLat))

	// Rebuild elevation offsets (EdgeElevOff/Prof)
	var newElevProf []int32
	newElevOff := make([]uint32, len(newEdges)+1)
	if len(g.EdgeElevOff) > 0 && len(g.EdgeElevProf) > 0 {
		for newIdx, oldIdx := range keptOldPositions {
			newElevOff[newIdx] = uint32(len(newElevProf))
			eStart := g.EdgeElevOff[oldIdx]
			eEnd := g.EdgeElevOff[oldIdx+1]
			if eEnd > eStart {
				newElevProf = append(newElevProf, g.EdgeElevProf[eStart:eEnd]...)
			}
		}
	}
	newElevOff[len(newEdges)] = uint32(len(newElevProf))

	newG.EdgeCount = uint32(len(newEdges))
	newG.Offsets = newOffsets
	newG.Targets = newEdges
	newG.EdgeDist = newDist
	newG.EdgeTags1 = newTags1
	newG.EdgeTags2 = newTags2
	newG.EdgeOSMIdx = newOSMIdx
	newG.OSMIDPool = g.OSMIDPool
	newG.EdgeGeomOff = newGeomOff
	newG.EdgeGeomLat = newGeomLat
	newG.EdgeGeomLon = newGeomLon
	newG.EdgeElevOff = newElevOff
	newG.EdgeElevProf = newElevProf

	return newG
}
