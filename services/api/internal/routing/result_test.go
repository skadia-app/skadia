package routing

import "testing"

func TestRouteResultAvgFlat(t *testing.T) {
	r := &RouteResult{DistSum: 1000, QualitySum: 1200}
	got := r.AvgFlat()
	if got != 1.2 {
		t.Errorf("AvgFlat = %v, want 1.2", got)
	}
}

func TestRouteResultAvgFlatEmpty(t *testing.T) {
	r := &RouteResult{}
	if r.AvgFlat() != 0 {
		t.Errorf("empty route AvgFlat = %v, want 0", r.AvgFlat())
	}
}
