package graph

import "testing"

func TestPruneWCCRemovesIsland(t *testing.T) {
	g := &Graph{
		NodeCount: 6,
		EdgeCount: 8,
		Offsets:   []uint32{0, 2, 4, 6, 8, 8, 8},
		Targets:   []uint32{1, 2, 0, 3, 0, 3, 1, 2},
		NodeLat:   []int32{470000000, 470010000, 470020000, 470030000, 480000000, 480010000},
		NodeLon:   []int32{110000000, 110000000, 110000000, 110000000, 120000000, 120000000},
		NodeEle:   []int32{5000, 5100, 5200, 5300, 1000, 1100},
		NodeTags:  []uint8{0, 0, 0, 0, 0, 0},
		EdgeDist:  []uint16{100, 100, 100, 100, 100, 100, 100, 100},
		EdgeTags1: make([]uint64, 8),
		EdgeTags2: make([]uint64, 8),
		EdgeOSMIdx: []uint32{0, 0, 0, 0, 0, 0, 0, 0},
		OSMIDPool: []int64{100},
	}
	g.Offsets = []uint32{0, 2, 4, 6, 8, 9, 10}
	g.Targets = []uint32{1, 2, 0, 3, 0, 3, 1, 2, 5, 4}
	g.EdgeCount = 10
	g.EdgeDist = make([]uint16, 10)
	g.EdgeTags1 = make([]uint64, 10)
	g.EdgeTags2 = make([]uint64, 10)
	g.EdgeOSMIdx = make([]uint32, 10)
	for i := range g.EdgeDist {
		g.EdgeDist[i] = 100
	}

	pruned := PruneWCC(g, 100)

	if pruned.NodeCount != 4 {
		t.Errorf("node count after prune: got %d, want 4", pruned.NodeCount)
	}
	for i := uint32(0); i < pruned.EdgeCount; i++ {
		if pruned.Targets[i] >= pruned.NodeCount {
			t.Errorf("edge %d target %d >= NodeCount %d", i, pruned.Targets[i], pruned.NodeCount)
		}
	}
}

func TestPruneWCCKeepsLargeComponents(t *testing.T) {
	g := &Graph{
		NodeCount: 5,
		EdgeCount: 8,
		Offsets:   []uint32{0, 2, 4, 6, 7, 8},
		Targets:   []uint32{1, 4, 0, 2, 1, 3, 2, 0},
		NodeLat:   []int32{470000000, 470010000, 470020000, 470030000, 470040000},
		NodeLon:   []int32{110000000, 110000000, 110000000, 110000000, 110000000},
		NodeEle:   []int32{5000, 5100, 5200, 5300, 5400},
		NodeTags:  []uint8{0, 0, 0, 0, 0},
		EdgeDist:  make([]uint16, 8),
		EdgeTags1: make([]uint64, 8),
		EdgeTags2: make([]uint64, 8),
		EdgeOSMIdx: make([]uint32, 8),
		OSMIDPool: []int64{100},
	}

	pruned := PruneWCC(g, 100)

	if pruned.NodeCount != 5 {
		t.Errorf("node count: got %d, want 5 (nothing should be pruned)", pruned.NodeCount)
	}
}

func TestPruneWCCOnewayConnectivity(t *testing.T) {
	g := &Graph{
		NodeCount: 3,
		EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2, 2},
		Targets:   []uint32{1, 2},
		NodeLat:   []int32{470000000, 470010000, 470020000},
		NodeLon:   []int32{110000000, 110000000, 110000000},
		NodeEle:   []int32{5000, 5100, 5200},
		NodeTags:  []uint8{0, 0, 0},
		EdgeDist:  []uint16{100, 100},
		EdgeTags1: make([]uint64, 2),
		EdgeTags2: make([]uint64, 2),
		EdgeOSMIdx: []uint32{0, 0},
		OSMIDPool: []int64{100},
	}

	pruned := PruneWCC(g, 2)

	if pruned.NodeCount != 3 {
		t.Errorf("oneway chain should stay connected: got %d nodes, want 3", pruned.NodeCount)
	}
}
