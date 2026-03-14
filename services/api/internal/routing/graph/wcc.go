package graph

// PruneWCC removes weakly connected components smaller than minSize.
// Uses union-find on the undirected version of the graph.
func PruneWCC(g *Graph, minSize int) *Graph {
	n := int(g.NodeCount)
	parent := make([]int, n)
	rank := make([]int, n)
	size := make([]int, n)
	for i := range parent {
		parent[i] = i
		size[i] = 1
	}

	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}

	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra == rb {
			return
		}
		if rank[ra] < rank[rb] {
			ra, rb = rb, ra
		}
		parent[rb] = ra
		size[ra] += size[rb]
		if rank[ra] == rank[rb] {
			rank[ra]++
		}
	}

	for i := uint32(0); i < g.NodeCount; i++ {
		for _, j := range g.Edges(i) {
			union(int(i), int(j))
		}
	}

	// Find the largest component size.
	maxSize := 0
	for i := 0; i < n; i++ {
		root := find(i)
		if size[root] > maxSize {
			maxSize = size[root]
		}
	}

	keep := make([]bool, n)
	for i := 0; i < n; i++ {
		root := find(i)
		// Keep a node if its component is >= minSize, OR if it belongs to
		// the largest component (always keep the main graph body).
		if size[root] >= minSize || size[root] == maxSize {
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
	for i := 0; i < n; i++ {
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
	for i := 0; i < n; i++ {
		if keep[i] {
			j := remap[i]
			newG.NodeLat[j] = g.NodeLat[i]
			newG.NodeLon[j] = g.NodeLon[i]
			newG.NodeEle[j] = g.NodeEle[i]
			newG.NodeTags[j] = g.NodeTags[i]
		}
	}

	var newEdges []uint32
	var newDist []uint16
	var newTags1, newTags2 []uint64
	var newOSMIdx []uint32
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
		}
		newOffsets[newNode+1] = uint32(len(newEdges))
	}

	newG.EdgeCount = uint32(len(newEdges))
	newG.Offsets = newOffsets
	newG.Targets = newEdges
	newG.EdgeDist = newDist
	newG.EdgeTags1 = newTags1
	newG.EdgeTags2 = newTags2
	newG.EdgeOSMIdx = newOSMIdx
	newG.OSMIDPool = g.OSMIDPool

	return newG
}
