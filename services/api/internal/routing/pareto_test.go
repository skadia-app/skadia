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
