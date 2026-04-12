package routing

import (
	"testing"
	"unsafe"
)

func TestParetoStateSize(t *testing.T) {
	size := unsafe.Sizeof(paretoState{})
	if size < 80 || size > 120 {
		t.Errorf("paretoState size = %d bytes, expected 80-120", size)
	}
	t.Logf("paretoState size: %d bytes", size)
}

func TestParetoStateDominance(t *testing.T) {
	a := paretoState{cost: 100, ascent: 50}
	b := paretoState{cost: 200, ascent: 100}
	if !a.dominates(b) {
		t.Error("a should dominate b")
	}
	if b.dominates(a) {
		t.Error("b should not dominate a")
	}

	c := paretoState{cost: 100, ascent: 50}
	if a.dominates(c) {
		t.Error("equal states should not dominate")
	}
	if c.dominates(a) {
		t.Error("equal states should not dominate (symmetric)")
	}

	d := paretoState{cost: 100, ascent: 100}
	e := paretoState{cost: 150, ascent: 50}
	if d.dominates(e) || e.dominates(d) {
		t.Error("Pareto-optimal pair should not dominate each other")
	}
}

func TestTryInsertDominated(t *testing.T) {
	rs := newReachableState()
	nodeID := uint64(42)

	_, ok := rs.tryInsert(nodeID, paretoState{cost: 100, ascent: 50})
	if !ok {
		t.Fatal("first insert should succeed")
	}

	_, ok = rs.tryInsert(nodeID, paretoState{cost: 200, ascent: 100})
	if ok {
		t.Error("dominated state should be rejected")
	}

	front := rs.fronts[nodeID]
	if len(front.activeIDs) != 1 {
		t.Errorf("front should have 1 active state, got %d", len(front.activeIDs))
	}
	if len(rs.slab.states) != 1 {
		t.Errorf("slab should have 1 state, got %d", len(rs.slab.states))
	}
}

func TestTryInsertRemovesDominated(t *testing.T) {
	rs := newReachableState()
	nodeID := uint64(42)

	_, _ = rs.tryInsert(nodeID, paretoState{cost: 100, ascent: 100})
	_, _ = rs.tryInsert(nodeID, paretoState{cost: 150, ascent: 50})

	front := rs.fronts[nodeID]
	if len(front.activeIDs) != 2 {
		t.Fatalf("after 2 incomparable inserts, front should have 2, got %d", len(front.activeIDs))
	}

	_, ok := rs.tryInsert(nodeID, paretoState{cost: 80, ascent: 40})
	if !ok {
		t.Fatal("dominant insert should succeed")
	}

	if len(front.activeIDs) != 1 {
		t.Errorf("after dominant insert, front should have 1, got %d", len(front.activeIDs))
	}
	if len(rs.slab.states) != 3 {
		t.Errorf("slab should have 3 states, got %d", len(rs.slab.states))
	}
}

func TestTryInsertSortedOrder(t *testing.T) {
	rs := newReachableState()
	nodeID := uint64(42)

	for _, cost := range []float32{300, 100, 200} {
		_, _ = rs.tryInsert(nodeID, paretoState{cost: cost, ascent: 1000 - cost})
	}

	front := rs.fronts[nodeID]
	if len(front.activeIDs) != 3 {
		t.Fatalf("expected 3 active states, got %d", len(front.activeIDs))
	}

	for i := 1; i < len(front.activeIDs); i++ {
		prev := rs.slab.states[front.activeIDs[i-1]]
		cur := rs.slab.states[front.activeIDs[i]]
		if prev.cost > cur.cost {
			t.Errorf("front not sorted: [%d].cost=%f > [%d].cost=%f",
				i-1, prev.cost, i, cur.cost)
		}
	}
}

func TestParetoFrontierHeapOrder(t *testing.T) {
	f := paretoFrontier{entries: make([]paretoFrontierEntry, 0, 16)}
	costs := []float32{50, 10, 30, 20, 40}
	for i, c := range costs {
		f.Push(paretoFrontierEntry{
			nodeID:  uint64(i),
			stateID: uint32(i),
			cost:    c,
		})
	}
	prev := float32(0)
	for f.Len() > 0 {
		e := f.Pop()
		if e.cost < prev {
			t.Errorf("heap order violated: %f < %f", e.cost, prev)
		}
		prev = e.cost
	}
}
