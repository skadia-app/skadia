package graph

import "testing"

func TestBuildReverseAdj(t *testing.T) {
	g := &Graph{
		NodeCount: 3,
		EdgeCount: 3,
		Offsets:   []uint32{0, 2, 3, 3}, // Node 0→1, 0→2, 1→2
		Targets:   []uint32{1, 2, 2},
	}
	g.EnsureReverseAdj()

	// Node 0: no incoming edges
	if g.RevOffsets[1]-g.RevOffsets[0] != 0 {
		t.Errorf("node 0 should have 0 incoming, got %d", g.RevOffsets[1]-g.RevOffsets[0])
	}
	// Node 1: 1 incoming (from 0)
	if g.RevOffsets[2]-g.RevOffsets[1] != 1 {
		t.Errorf("node 1 should have 1 incoming, got %d", g.RevOffsets[2]-g.RevOffsets[1])
	}
	if g.RevTargets[g.RevOffsets[1]] != 0 {
		t.Errorf("node 1 incoming should be from node 0, got %d", g.RevTargets[g.RevOffsets[1]])
	}
	// Node 2: 2 incoming (from 0 and 1)
	if g.RevOffsets[3]-g.RevOffsets[2] != 2 {
		t.Errorf("node 2 should have 2 incoming, got %d", g.RevOffsets[3]-g.RevOffsets[2])
	}
}

func TestBuildReverseAdjEmpty(t *testing.T) {
	g := &Graph{NodeCount: 0, EdgeCount: 0}
	g.EnsureReverseAdj()
	if g.RevOffsets != nil {
		t.Error("empty graph should have nil RevOffsets")
	}
}

func TestBuildReverseAdjBidirectional(t *testing.T) {
	// 0↔1↔2: each edge exists in both directions
	g := &Graph{
		NodeCount: 3,
		EdgeCount: 4,
		Offsets:   []uint32{0, 1, 3, 4},
		Targets:   []uint32{1, 0, 2, 1},
	}
	g.EnsureReverseAdj()

	// Each node should have the expected incoming count
	for i, want := range []uint32{1, 2, 1} {
		got := g.RevOffsets[i+1] - g.RevOffsets[i]
		if got != want {
			t.Errorf("node %d: incoming=%d, want %d", i, got, want)
		}
	}
}
