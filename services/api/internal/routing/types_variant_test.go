package routing

import "testing"

// TestMinVariantDistanceM checks boundary conditions.
func TestMinVariantDistanceM(t *testing.T) {
	// Floor: short distances must still produce ≥300 m
	if d := MinVariantDistanceM(0); d != 300.0 {
		t.Errorf("MinVariantDistanceM(0) = %.1f, want 300.0", d)
	}
	if d := MinVariantDistanceM(1000); d != 300.0 {
		t.Errorf("MinVariantDistanceM(1000) = %.1f, want 300.0 (5%% = 50 m < floor)", d)
	}
	// Above floor: 5% of 10 km = 500 m
	if d := MinVariantDistanceM(10000); d != 500.0 {
		t.Errorf("MinVariantDistanceM(10000) = %.1f, want 500.0", d)
	}
}
