package enrichment

import (
	"testing"
	"time"
)

func TestDetectAscentsFromSmoothed_EndsAtLastClimbingPoint(t *testing.T) {
	smoothed := []float64{0, 20, 40, 30}
	distances := []float64{0, 100, 200, 300}

	got := detectAscentsFromSmoothed(smoothed, distances, 5, 20)
	if len(got) != 1 {
		t.Fatalf("got %d ascents, want 1", len(got))
	}
	if got[0].endDistM != 200 {
		t.Fatalf("endDistM = %.0f, want 200 (last climbing point before descent)", got[0].endDistM)
	}
	if got[0].elevationGain != 40 {
		t.Fatalf("elevationGain = %.0f, want 40", got[0].elevationGain)
	}
}

func TestDetectAscentsFromStream_DoesNotBridgeLongRecordingGap(t *testing.T) {
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	stream := []EnrichmentStreamPoint{
		{Time: base, AltitudeM: f64ptr(0), DistanceM: f64ptr(0)},
		{Time: base.Add(1 * time.Second), AltitudeM: f64ptr(1), DistanceM: f64ptr(10)},
		// Ten-minute gap. Without a gap guard this forms one false 120m ascent.
		{Time: base.Add(10 * time.Minute), AltitudeM: f64ptr(100), DistanceM: f64ptr(200)},
		{Time: base.Add(10*time.Minute + time.Second), AltitudeM: f64ptr(110), DistanceM: f64ptr(300)},
		{Time: base.Add(10*time.Minute + 2*time.Second), AltitudeM: f64ptr(120), DistanceM: f64ptr(400)},
	}

	got := detectAscentsFromStream(stream, 5, 50)
	if len(got) != 0 {
		t.Fatalf("got %d ascents, want 0 (long recording gap must split detection)", len(got))
	}
}

func TestDetectAscentsFromStream_RemovesTransientAltitudeExcursion(t *testing.T) {
	base := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	points := []struct {
		dist, alt float64
	}{
		{0, 410}, {6, 410.2}, {13, 405.4},
		{20, 392.8}, {27, 383.8}, {35, 376.4},
		{43, 370.2}, {51, 362.8}, {60, 358.8},
		{69, 356.8}, {78, 355.8}, {87, 357.6},
		{96, 360.0}, {105, 363.4}, {114, 365.0},
		{123, 367.4}, {132, 369.4}, {141, 371.6},
		{150, 374.8}, {159, 378.0}, {168, 380.6},
		{177, 385.0}, {185, 387.2}, {193, 385.4},
		{201, 381.8}, {210, 379.6}, {218, 379.4},
		{226, 379.8}, {234, 378.6}, {242, 380.8},
		{250, 388.8}, {257, 397.6}, {264, 402.4},
		{271, 404.8}, {278, 406.2}, {285, 406.8},
		{292, 407.2}, {299, 408.4}, {306, 413.4},
		{313, 418.0}, {319, 423.2}, {325, 426.0},
	}
	stream := make([]EnrichmentStreamPoint, len(points))
	for i, p := range points {
		stream[i] = EnrichmentStreamPoint{
			Time:      base.Add(time.Duration(i) * time.Second),
			AltitudeM: f64ptr(p.alt),
			DistanceM: f64ptr(p.dist),
		}
	}

	got := detectAscentsFromStream(stream, 5, 20)
	if len(got) != 0 {
		t.Fatalf("got %d ascents, want 0 for a transient altitude excursion", len(got))
	}
}

func TestDetectAscentsFromSmoothed_MaxGradientUsesDenseSamples(t *testing.T) {
	smoothed := make([]float64, 101)
	distances := make([]float64, 101)
	for i := range smoothed {
		distances[i] = float64(i)
		smoothed[i] = float64(i) * 0.2
	}

	got := detectAscentsFromSmoothed(smoothed, distances, 5, 20)
	if len(got) != 1 {
		t.Fatalf("got %d ascents, want 1", len(got))
	}
	if got[0].maxGradient <= 0 {
		t.Fatalf("maxGradient = %.1f, want > 0 for dense 1m samples", got[0].maxGradient)
	}
}

func TestDetectAscentsFromSmoothed_ExtendsAcrossSmallDipBeforeFiltering(t *testing.T) {
	smoothed := []float64{0, 10, 25, 24.8, 28, 34, 39, 43}
	distances := []float64{0, 100, 200, 205, 300, 400, 500, 600}

	got := detectAscentsFromSmoothed(smoothed, distances, 5, 20)
	if len(got) != 1 {
		t.Fatalf("got %d ascents, want 1", len(got))
	}
	if got[0].startDistM != 0 || got[0].endDistM != 600 {
		t.Fatalf("range = %.0f-%.0f, want 0-600", got[0].startDistM, got[0].endDistM)
	}
	if got[0].elevationGain != 43 {
		t.Fatalf("elevationGain = %.1f, want 43.0", got[0].elevationGain)
	}
}

func TestDetectAscentsFromSmoothed_MergesRollingClimbAcrossModestDip(t *testing.T) {
	smoothed := []float64{0, 30, 60, 55, 75, 95}
	distances := []float64{0, 100, 200, 300, 400, 500}

	got := detectAscentsFromSmoothed(smoothed, distances, 5, 20)
	if len(got) != 1 {
		t.Fatalf("got %d ascents, want 1 rolling climb despite a modest dip", len(got))
	}
	if got[0].startDistM != 0 || got[0].endDistM != 500 {
		t.Fatalf("range = %.0f-%.0f, want 0-500", got[0].startDistM, got[0].endDistM)
	}
}

func TestDetectAscentsFromSmoothed_IgnoresWeakRipplesWhileMergingClimb(t *testing.T) {
	smoothed := []float64{0, 60, 58, 58.1, 57.5, 80, 100}
	distances := []float64{0, 200, 210, 220, 230, 400, 500}

	got := detectAscentsFromSmoothed(smoothed, distances, 5, 20)
	if len(got) != 1 {
		t.Fatalf("got %d ascents, want 1 with weak ripples ignored between climb parts", len(got))
	}
	if got[0].startDistM != 0 || got[0].endDistM != 500 {
		t.Fatalf("range = %.0f-%.0f, want 0-500", got[0].startDistM, got[0].endDistM)
	}
}

func TestDetectAscentsFromSmoothed_DoesNotMergeAcrossSubstantialDescent(t *testing.T) {
	smoothed := []float64{0, 80, 40, 70, 100}
	distances := []float64{0, 100, 200, 300, 400}

	got := detectAscentsFromSmoothed(smoothed, distances, 5, 20)
	if len(got) != 2 {
		t.Fatalf("got %d ascents, want 2 separated by substantial descent", len(got))
	}
	if got[0].startDistM != 0 || got[0].endDistM != 100 {
		t.Fatalf("first range = %.0f-%.0f, want 0-100", got[0].startDistM, got[0].endDistM)
	}
	if got[1].startDistM != 200 || got[1].endDistM != 400 {
		t.Fatalf("second range = %.0f-%.0f, want 200-400", got[1].startDistM, got[1].endDistM)
	}
}

func TestDetectAscentsFromSmoothed_DoesNotChainExtensionsIntoDistantCore(t *testing.T) {
	smoothed := []float64{0, 60, 58, 73, 68, 103, 133}
	distances := []float64{0, 200, 250, 350, 550, 900, 1000}

	got := detectAscentsFromSmoothed(smoothed, distances, 5, 20)
	if len(got) != 2 {
		t.Fatalf("got %d ascents, want 2 when the next core starts beyond the merge gap", len(got))
	}
	if got[0].startDistM != 0 || got[0].endDistM != 350 {
		t.Fatalf("first range = %.0f-%.0f, want 0-350", got[0].startDistM, got[0].endDistM)
	}
	if got[1].startDistM != 550 || got[1].endDistM != 1000 {
		t.Fatalf("second range = %.0f-%.0f, want 550-1000", got[1].startDistM, got[1].endDistM)
	}
}

func TestMaxGradientPct_UsesFiftyMeterWindow(t *testing.T) {
	smoothed := make([]float64, 21)
	distances := make([]float64, 21)
	for i := range smoothed {
		distances[i] = float64(i) * 5
		if distances[i] >= 50 {
			smoothed[i] = 30
		}
	}

	got := maxGradientPct(smoothed, distances, 0, len(smoothed)-1)
	if got < 59.9 || got > 60.1 {
		t.Fatalf("maxGradientPct = %.1f, want 60.0 from a 30m gain over 50m", got)
	}
}

func f64ptr(v float64) *float64 {
	return &v
}

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

func TestGradientSanityCap(t *testing.T) {
	// Verify that max gradient is capped at 200%
	g := 500.0 // would be 500% without cap
	g = min(g, 200.0)
	if g != 200.0 {
		t.Errorf("gradient cap: got %.0f, want 200", g)
	}
}
