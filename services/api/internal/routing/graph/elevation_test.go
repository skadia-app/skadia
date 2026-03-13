package graph

import "testing"

func TestInterpolateBridgeElevation(t *testing.T) {
	elevations := []int32{5000, 0, 0, 0, 4500}
	isBridge := []bool{false, true, true, true, false}

	InterpolateMissing(elevations, isBridge)

	if elevations[1] != 4875 {
		t.Errorf("node 1: got %d, want 4875", elevations[1])
	}
	if elevations[2] != 4750 {
		t.Errorf("node 2: got %d, want 4750", elevations[2])
	}
	if elevations[3] != 4625 {
		t.Errorf("node 3: got %d, want 4625", elevations[3])
	}
}

func TestInterpolateLeadingMissing(t *testing.T) {
	elevations := []int32{0, 0, 3000, 4000}
	isBridge := []bool{true, true, false, false}

	InterpolateMissing(elevations, isBridge)

	if elevations[0] != 3000 {
		t.Errorf("node 0: got %d, want 3000", elevations[0])
	}
	if elevations[1] != 3000 {
		t.Errorf("node 1: got %d, want 3000", elevations[1])
	}
}

func TestInterpolateTrailingMissing(t *testing.T) {
	elevations := []int32{2000, 3000, 0, 0}
	isBridge := []bool{false, false, true, true}

	InterpolateMissing(elevations, isBridge)

	if elevations[2] != 3000 {
		t.Errorf("node 2: got %d, want 3000", elevations[2])
	}
	if elevations[3] != 3000 {
		t.Errorf("node 3: got %d, want 3000", elevations[3])
	}
}
