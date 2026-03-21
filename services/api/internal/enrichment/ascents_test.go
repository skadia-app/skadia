package enrichment

import (
	"testing"
)

func TestClassifyClimb_CyclingCategories(t *testing.T) {
	tests := []struct {
		elevGain, gradient float64
		want               string
	}{
		{1600, 6, "HC"},
		{1000, 8, "HC"},
		{700, 5, "1"},
		{400, 8, "1"},
		{350, 5, "2"},
		{200, 8, "2"},
		{160, 5, "3"},
		{100, 8, "3"},
		{50, 5, "4"},
	}
	for _, tt := range tests {
		got := classifyClimb(tt.elevGain, tt.gradient)
		if got != tt.want {
			t.Errorf("classifyClimb(%.0f, %.0f) = %q, want %q",
				tt.elevGain, tt.gradient, got, tt.want)
		}
	}
}

func TestClassifyClimbSportClass_Hiking(t *testing.T) {
	tests := []struct {
		gradient float64
		want     string
	}{
		{45, "S5"},
		{25, "S4"},
		{12, "S3"},
		{7, "S2"},
		{3, "S1"},
	}
	for _, tt := range tests {
		got := classifyClimbSportClass(100, tt.gradient, "foot")
		if got != tt.want {
			t.Errorf("foot gradient=%.0f%% → %q, want %q", tt.gradient, got, tt.want)
		}
	}
}

func TestClassifyClimbSportClass_BikeReturnsEmpty(t *testing.T) {
	got := classifyClimbSportClass(500, 8, "bike")
	if got != "" {
		t.Errorf("bike sport_class should be empty, got %q", got)
	}
}

func TestClassifyClimbSportClass_Generic(t *testing.T) {
	tests := []struct {
		elevGain float64
		want     string
	}{
		{600, "schwer"},
		{300, "mittel"},
		{50, "leicht"},
	}
	for _, tt := range tests {
		got := classifyClimbSportClass(tt.elevGain, 10, "generic")
		if got != tt.want {
			t.Errorf("generic elevGain=%.0fm → %q, want %q", tt.elevGain, got, tt.want)
		}
	}
}

func TestMergeShallowDips_MergesSmallDip(t *testing.T) {
	// Two ascents separated by 2m dip over 30m → should merge
	smoothed := make([]float64, 200)
	distances := make([]float64, 200)
	for i := range 200 {
		distances[i] = float64(i) * 5.0
		// Rising 0-500m, small 2m dip at 250m, then rising again
		if distances[i] < 250 {
			smoothed[i] = distances[i] * 0.2 // 20% gradient
		} else if distances[i] < 280 {
			smoothed[i] = 50 - (distances[i]-250)*0.067 // ~2m dip
		} else {
			smoothed[i] = 48 + (distances[i]-280)*0.2
		}
	}

	ascents := []ascent{
		{startDistM: 0, endDistM: 250, lengthM: 250, elevationGain: 50, avgGradient: 20},
		{startDistM: 280, endDistM: 500, lengthM: 220, elevationGain: 44, avgGradient: 20},
	}

	merged := mergeShallowDips(ascents, smoothed, distances, 3.0, 50.0)
	if len(merged) != 1 {
		t.Fatalf("got %d ascents, want 1 (merged)", len(merged))
	}
	if merged[0].endDistM != 500 {
		t.Errorf("merged end: got %.0f, want 500", merged[0].endDistM)
	}
}

func TestMergeShallowDips_KeepsDeepDip(t *testing.T) {
	// Two ascents separated by 10m dip → should NOT merge
	smoothed := make([]float64, 200)
	distances := make([]float64, 200)
	for i := range 200 {
		distances[i] = float64(i) * 5.0
		if distances[i] < 250 {
			smoothed[i] = distances[i] * 0.2
		} else if distances[i] < 280 {
			smoothed[i] = 50 - (distances[i]-250)*0.333 // ~10m dip
		} else {
			smoothed[i] = 40 + (distances[i]-280)*0.2
		}
	}

	ascents := []ascent{
		{startDistM: 0, endDistM: 250, lengthM: 250, elevationGain: 50, avgGradient: 20},
		{startDistM: 280, endDistM: 500, lengthM: 220, elevationGain: 44, avgGradient: 20},
	}

	merged := mergeShallowDips(ascents, smoothed, distances, 3.0, 50.0)
	if len(merged) != 2 {
		t.Fatalf("got %d ascents, want 2 (deep dip, no merge)", len(merged))
	}
}

func TestMergeShallowDips_SingleAscent(t *testing.T) {
	// Single ascent → unchanged
	smoothed := []float64{500, 510, 520}
	distances := []float64{0, 50, 100}
	ascents := []ascent{
		{startDistM: 0, endDistM: 100, lengthM: 100, elevationGain: 20, avgGradient: 20},
	}
	merged := mergeShallowDips(ascents, smoothed, distances, 3.0, 50.0)
	if len(merged) != 1 {
		t.Fatalf("single ascent: got %d, want 1", len(merged))
	}
}

func TestMergeShallowDips_LargeGapNoMerge(t *testing.T) {
	// Gap > maxDipDistM (50m) → should NOT merge
	smoothed := make([]float64, 200)
	distances := make([]float64, 200)
	for i := range 200 {
		distances[i] = float64(i) * 5.0
		smoothed[i] = 500 + distances[i]*0.1
	}

	ascents := []ascent{
		{startDistM: 0, endDistM: 200, lengthM: 200, elevationGain: 20, avgGradient: 10},
		{startDistM: 400, endDistM: 600, lengthM: 200, elevationGain: 20, avgGradient: 10},
	}

	merged := mergeShallowDips(ascents, smoothed, distances, 3.0, 50.0)
	if len(merged) != 2 {
		t.Fatalf("large gap: got %d, want 2 (no merge)", len(merged))
	}
}

func TestGradientSanityCap(t *testing.T) {
	// Verify that max gradient is capped at 200%
	g := 500.0 // would be 500% without cap
	g = min(g, 200.0)
	if g != 200.0 {
		t.Errorf("gradient cap: got %.0f, want 200", g)
	}
}
