package routing

import "testing"

func TestReconstructPath_ShortChain(t *testing.T) {
	rs := newReachableState()

	rootID, _ := rs.tryInsert(1, paretoState{
		cost:          0,
		parentStateID: rootStateID,
	})
	midID, _ := rs.tryInsert(2, paretoState{
		cost:          100,
		handle:        uint64(1001),
		parentStateID: rootID,
	})
	targetID, _ := rs.tryInsert(3, paretoState{
		cost:          200,
		handle:        uint64(1002),
		parentStateID: midID,
	})

	edges, target, err := ReconstructPath(rs, targetID)
	if err != nil {
		t.Fatalf("ReconstructPath: %v", err)
	}
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
	if len(edges) >= 2 && (edges[0] != 1001 || edges[1] != 1002) {
		t.Errorf("edge order wrong: %v", edges)
	}
	if target == nil || target.cost != 200 {
		t.Errorf("target state wrong")
	}
}

func TestReconstructPath_OutOfRange(t *testing.T) {
	rs := newReachableState()
	_, _, err := ReconstructPath(rs, 99999)
	if err == nil {
		t.Error("expected error for out-of-range stateID")
	}
}

func TestReconstructPath_MaxHopsSafeguard(t *testing.T) {
	rs := newReachableState()
	// Build a cyclic parent chain (programmer error simulation)
	rs.slab.states = append(rs.slab.states, paretoState{parentStateID: 1})
	rs.slab.states = append(rs.slab.states, paretoState{parentStateID: 0})

	_, _, err := ReconstructPath(rs, 0)
	if err == nil {
		t.Error("expected error for cyclic chain")
	}
}

func TestComputeWayIDOverlap(t *testing.T) {
	a := map[int64]bool{1: true, 2: true, 3: true}
	b := map[int64]bool{2: true, 3: true, 4: true}
	got := ComputeWayIDOverlap(a, b)
	want := 2.0 / 3.0
	if got < want-0.001 || got > want+0.001 {
		t.Errorf("overlap = %v, want ~%v", got, want)
	}
}

func TestComputeWayIDOverlap_Empty(t *testing.T) {
	if ComputeWayIDOverlap(nil, map[int64]bool{1: true}) != 0 {
		t.Error("empty set should give 0 overlap")
	}
}

func TestComputeWayIDOverlap_Identical(t *testing.T) {
	a := map[int64]bool{1: true, 2: true}
	if ComputeWayIDOverlap(a, a) != 1.0 {
		t.Errorf("identical sets should give 1.0 overlap")
	}
}
