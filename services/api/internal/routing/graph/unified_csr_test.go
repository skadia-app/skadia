package graph

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBuildUnifiedCSR_BasicMerge(t *testing.T) {
	// Create a temp dir with 2 small synthetic tiles
	dir := t.TempDir()

	// Tile A: 3 nodes (0,1 are border), 2 edges: 0→1, 1→2
	tileA := &Graph{
		TileID:    100,
		NodeCount: 3,
		EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2, 2},
		Targets:   []uint32{1, 2},
		EdgeDist:  []uint32{100, 200},
		EdgeTags1: []uint64{0, 0},
		EdgeTags2: []uint64{0, 0},
		NodeLat:   []int32{470000000, 470000000, 470500000},
		NodeLon:   []int32{80000000, 81000000, 80500000},
		NodeEle:   []int32{500, 600, 700},
		IsBorder:  []bool{true, true, false},
	}
	// Build BorderIndex for tile A
	borderIDs := []uint32{0, 1}
	tileA.BorderIndex = BuildBorderNodeIndex(borderIDs, tileA.NodeLat, tileA.NodeLon)

	// Tile B: 3 nodes (0,1 are border — node 0 matches tile A node 1)
	tileB := &Graph{
		TileID:    101,
		NodeCount: 3,
		EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2, 2},
		Targets:   []uint32{1, 2},
		EdgeDist:  []uint32{150, 250},
		EdgeTags1: []uint64{0, 0},
		EdgeTags2: []uint64{0, 0},
		NodeLat:   []int32{470000000, 470000000, 470500000}, // node 0 matches tile A node 1
		NodeLon:   []int32{81000000, 82000000, 81500000},    // same lon as tile A node 1
		NodeEle:   []int32{600, 800, 750},
		IsBorder:  []bool{true, true, false},
	}
	borderIDs = []uint32{0, 1}
	tileB.BorderIndex = BuildBorderNodeIndex(borderIDs, tileB.NodeLat, tileB.NodeLon)

	// Serialize tiles
	pathA := filepath.Join(dir, TileFileName(100))
	pathB := filepath.Join(dir, TileFileName(101))
	if err := Serialize(tileA, pathA); err != nil {
		t.Fatal(err)
	}
	if err := Serialize(tileB, pathB); err != nil {
		t.Fatal(err)
	}

	// Build unified CSR
	edgeCostFn := func(tags1, tags2 uint64) EdgeCostFactors {
		return EdgeCostFactors{Flat: 1.0}
	}
	csr, nodeMapping, _, err := BuildUnifiedCSR(context.Background(), dir, edgeCostFn, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer csr.Close()

	// Verify dedup: tile A node 1 and tile B node 0 share coordinates → same denseID
	// Total raw nodes = 6, but 1 border node is deduped → 5 unique nodes
	if csr.NodeCount != 5 {
		t.Errorf("expected 5 nodes (1 deduped), got %d", csr.NodeCount)
	}

	// Verify edges: 4 total (2 from tile A + 2 from tile B)
	if csr.EdgeCount != 4 {
		t.Errorf("expected 4 edges, got %d", csr.EdgeCount)
	}

	// Verify Offsets consistency
	if csr.Offsets[csr.NodeCount] != csr.EdgeCount {
		t.Errorf("Offsets[NodeCount]=%d != EdgeCount=%d", csr.Offsets[csr.NodeCount], csr.EdgeCount)
	}

	// Verify deduped node has edges from BOTH tiles
	mappingA := nodeMapping[100]
	mappingB := nodeMapping[101]
	dedupDenseID := mappingA[1] // tile A node 1
	if mappingB[0] != dedupDenseID {
		t.Errorf("border dedup failed: tile A node 1 = %d, tile B node 0 = %d", mappingA[1], mappingB[0])
	}

	// The deduped node should have edges from both tiles
	edgeCount := csr.Offsets[dedupDenseID+1] - csr.Offsets[dedupDenseID]
	if edgeCount != 2 {
		t.Errorf("deduped border node should have 2 edges (1 from each tile), got %d", edgeCount)
	}
}

func TestBuildUnifiedCSR_EdgeCountConsistency(t *testing.T) {
	// Reuse the BasicMerge setup, then traverse all edges to check no OOB
	dir := t.TempDir()
	tileA := &Graph{
		TileID: 100, NodeCount: 2, EdgeCount: 1,
		Offsets:   []uint32{0, 1, 1},
		Targets:   []uint32{1},
		EdgeDist:  []uint32{100},
		EdgeTags1: []uint64{0},
		EdgeTags2: []uint64{0},
		NodeLat:   []int32{470000000, 470500000},
		NodeLon:   []int32{80000000, 80500000},
		NodeEle:   []int32{500, 600},
		IsBorder:  []bool{false, false},
	}
	if err := Serialize(tileA, filepath.Join(dir, TileFileName(100))); err != nil {
		t.Fatal(err)
	}

	csr, _, _, err := BuildUnifiedCSR(context.Background(), dir,
		func(t1, t2 uint64) EdgeCostFactors { return EdgeCostFactors{Flat: 1.0} }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer csr.Close()

	// Traverse all edges — must not panic
	for nd := range csr.NodeCount {
		for ei := csr.Offsets[nd]; ei < csr.Offsets[nd+1]; ei++ {
			if csr.Edges[ei].Target >= csr.NodeCount {
				t.Fatalf("edge target %d >= NodeCount %d", csr.Edges[ei].Target, csr.NodeCount)
			}
		}
	}
}
