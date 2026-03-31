// frontier_test.go
package routing

import (
	"math"
	"math/rand"
	"testing"
)

func TestFrontier_PopReturnsMinCost(t *testing.T) {
	var f Frontier
	f.Push(FrontierEntry{NodeID: 1, Cost: 5.0})
	f.Push(FrontierEntry{NodeID: 2, Cost: 2.0})
	f.Push(FrontierEntry{NodeID: 3, Cost: 8.0})
	f.Push(FrontierEntry{NodeID: 4, Cost: 1.0})

	got := f.Pop()
	if got.NodeID != 4 || got.Cost != 1.0 {
		t.Errorf("expected node 4 cost 1.0, got node %d cost %f", got.NodeID, got.Cost)
	}
}

func TestFrontier_PopOrder_1000(t *testing.T) {
	var f Frontier
	for i := range 1000 {
		f.Push(FrontierEntry{
			NodeID: uint64(i),
			Cost:   rand.Float32() * 10000,
		})
	}

	prev := float32(-1)
	for f.Len() > 0 {
		e := f.Pop()
		if e.Cost < prev {
			t.Fatalf("heap order violated: %f < %f", e.Cost, prev)
		}
		prev = e.Cost
	}
}

func TestFrontier_CostAffectsOrder(t *testing.T) {
	var f Frontier
	f.Push(FrontierEntry{NodeID: 1, Cost: 13.0}) // key=13
	f.Push(FrontierEntry{NodeID: 2, Cost: 6.0})  // key=6

	got := f.Pop()
	if got.NodeID != 2 {
		t.Errorf("expected node 2 (cost=6), got node %d", got.NodeID)
	}
}

func TestFrontier_Empty_Len(t *testing.T) {
	var f Frontier
	if f.Len() != 0 {
		t.Errorf("empty frontier should have len 0")
	}
	f.Push(FrontierEntry{Cost: 1})
	if f.Len() != 1 {
		t.Errorf("expected len 1, got %d", f.Len())
	}
	f.Pop()
	if f.Len() != 0 {
		t.Errorf("expected len 0 after pop, got %d", f.Len())
	}
}

func TestFrontier_Reset(t *testing.T) {
	var f Frontier
	for i := range 100 {
		f.Push(FrontierEntry{Cost: float32(i)})
	}
	f.Reset()
	if f.Len() != 0 {
		t.Errorf("expected len 0 after reset, got %d", f.Len())
	}
	// Ensure backing slice is retained (no realloc on next push)
	f.Push(FrontierEntry{Cost: 42})
	if f.Len() != 1 {
		t.Errorf("expected len 1 after re-push")
	}
}

func TestFrontierPeekCost(t *testing.T) {
	f := Frontier{entries: make([]FrontierEntry, 0, 8)}

	if got := f.PeekCost(); got != math.MaxFloat32 {
		t.Errorf("empty PeekCost: expected MaxFloat32, got %f", got)
	}

	f.Push(FrontierEntry{NodeID: 1, Cost: 5.0})
	f.Push(FrontierEntry{NodeID: 2, Cost: 3.0})
	f.Push(FrontierEntry{NodeID: 3, Cost: 7.0})

	if got := f.PeekCost(); got != 3.0 {
		t.Errorf("PeekCost: expected 3.0, got %f", got)
	}

	f.Pop()
	if got := f.PeekCost(); got != 5.0 {
		t.Errorf("after pop, PeekCost: expected 5.0, got %f", got)
	}
}

func BenchmarkFrontier_Push1M(b *testing.B) {
	entries := make([]FrontierEntry, 1_000_000)
	for i := range entries {
		entries[i] = FrontierEntry{
			NodeID: uint64(i),
			Cost:   rand.Float32() * 100000,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var f Frontier
		for _, e := range entries {
			f.Push(e)
		}
	}
}
