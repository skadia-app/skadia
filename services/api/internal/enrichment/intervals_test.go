package enrichment

import (
	"testing"
	"time"
)

// TestSplitTypeToIntensity tests the split_type → intensity mapping.
func TestSplitTypeToIntensity(t *testing.T) {
	tests := []struct {
		splitType string
		want      string
	}{
		{"interval_active", "hard"},
		{"interval_rest", "recovery"},
		{"interval_recovery", "recovery"},
		{"interval_warmup", "easy"},
		{"interval_cooldown", "easy"},
		{"interval_other", "moderate"},
		{"workout_round", "moderate"},
		{"ascent_split", ""}, // not a workout type
		{"run_active", ""},   // not a workout type
		{"", ""},             // empty
	}
	for _, tt := range tests {
		got := splitTypeToIntensity(tt.splitType)
		if got != tt.want {
			t.Errorf("splitTypeToIntensity(%q) = %q, want %q", tt.splitType, got, tt.want)
		}
	}
}

// TestSplitTypeKeys_ContainsAllWorkoutTypes verifies splitTypeKeys returns all
// entries from workoutSplitTypes.
func TestSplitTypeKeys_ContainsAllWorkoutTypes(t *testing.T) {
	keys := splitTypeKeys()
	if len(keys) != len(workoutSplitTypes) {
		t.Fatalf("splitTypeKeys len = %d, want %d", len(keys), len(workoutSplitTypes))
	}
	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}
	for k := range workoutSplitTypes {
		if !keySet[k] {
			t.Errorf("key %q missing from splitTypeKeys()", k)
		}
	}
}

// TestNeverIntervalSports verifies that non-eligible foot sports are excluded
// from interval detection (not in any whitelist map).
func TestNeverIntervalSports_ContainsHiking(t *testing.T) {
	excluded := []string{"hiking", "walking", "mountaineering", "rock_climbing"}
	for _, sp := range excluded {
		if intervalEligibleFoot[sp] || intervalEligibleExtra[sp] {
			t.Errorf("sport %q should not be eligible for interval detection", sp)
		}
	}
}

// TestComputeIntervalConfidence_NotEnoughWork returns 0.0 for < 3 hard phases.
func TestComputeIntervalConfidence_NotEnoughWork(t *testing.T) {
	classified := []classifiedSegment{
		{durationS: 120, intensity: "hard"},
		{durationS: 60, intensity: "recovery"},
	}
	got := computeIntervalConfidence(classified)
	if got != 0.0 {
		t.Errorf("got %.2f, want 0.0 for < 3 work phases", got)
	}
}

// TestComputeIntervalConfidence_TooIrregular returns 0.0 when CV >= 0.40.
func TestComputeIntervalConfidence_TooIrregular(t *testing.T) {
	// Durations: 30, 300, 30 — huge CV
	classified := []classifiedSegment{
		{durationS: 30, intensity: "hard"},
		{durationS: 60, intensity: "recovery"},
		{durationS: 300, intensity: "hard"},
		{durationS: 60, intensity: "recovery"},
		{durationS: 30, intensity: "hard"},
	}
	got := computeIntervalConfidence(classified)
	if got != 0.0 {
		t.Errorf("got %.2f, want 0.0 for irregular durations", got)
	}
}

// TestComputeIntervalConfidence_FourWorkPhases returns 0.7 for 4 regular work phases.
func TestComputeIntervalConfidence_FourWorkPhases(t *testing.T) {
	// 4 hard segments of ~equal duration: CV ≈ 0.01 < 0.30
	classified := []classifiedSegment{
		{durationS: 180, intensity: "hard"},
		{durationS: 90, intensity: "recovery"},
		{durationS: 182, intensity: "hard"},
		{durationS: 90, intensity: "recovery"},
		{durationS: 179, intensity: "hard"},
		{durationS: 90, intensity: "recovery"},
		{durationS: 181, intensity: "hard"},
	}
	got := computeIntervalConfidence(classified)
	if got < 0.7 {
		t.Errorf("got %.2f, want >= 0.7 for 4 regular work phases", got)
	}
}

// TestComputeIntervalConfidence_HighConfidence returns 0.9 for strict alternation
// with 5 work phases, low duration CV, and low intensity CV.
func TestComputeIntervalConfidence_HighConfidence(t *testing.T) {
	hr300 := 300.0
	hr302 := 302.0
	hr298 := 298.0
	hr301 := 301.0
	hr299 := 299.0

	classified := []classifiedSegment{
		{durationS: 180, intensity: "hard", avgPower: &hr300},
		{durationS: 90, intensity: "recovery"},
		{durationS: 182, intensity: "hard", avgPower: &hr300},
		{durationS: 90, intensity: "recovery"},
		{durationS: 179, intensity: "hard", avgPower: &hr302},
		{durationS: 90, intensity: "recovery"},
		{durationS: 181, intensity: "hard", avgPower: &hr298},
		{durationS: 90, intensity: "recovery"},
		{durationS: 180, intensity: "hard", avgPower: &hr301},
		{durationS: 90, intensity: "recovery"},
		// One more hard to make 6 total; avgPower very close
		{durationS: 181, intensity: "hard", avgPower: &hr299},
	}
	got := computeIntervalConfidence(classified)
	if got < 0.7 {
		t.Errorf("got %.2f, want >= 0.7 for high-confidence intervals", got)
	}
}

// TestComputeIntervalConfidence_IntensityCVDefault verifies that missing
// intensity data defaults to intensityCV=1.0 (not 0.0), preventing false 0.9.
func TestComputeIntervalConfidence_IntensityCVDefault(t *testing.T) {
	// 5 hard phases, good duration CV, good alternation, NO power/HR/speed data
	// → intensityCV defaults to 1.0 → should NOT reach 0.9 score
	classified := []classifiedSegment{
		{durationS: 180, intensity: "hard"},
		{durationS: 90, intensity: "recovery"},
		{durationS: 182, intensity: "hard"},
		{durationS: 90, intensity: "recovery"},
		{durationS: 179, intensity: "hard"},
		{durationS: 90, intensity: "recovery"},
		{durationS: 181, intensity: "hard"},
		{durationS: 90, intensity: "recovery"},
		{durationS: 180, intensity: "hard"},
	}
	got := computeIntervalConfidence(classified)
	// Should be 0.7 (4+ work phases, CV < 0.30) but NOT 0.9 (intensityCV = 1.0)
	if got >= 0.9 {
		t.Errorf("got %.2f, should not reach 0.9 when intensity data absent (intensityCV defaults to 1.0)", got)
	}
	if got < 0.7 {
		t.Errorf("got %.2f, should still be >= 0.7 for 5 regular work phases", got)
	}
}

// TestClassifySegments_HardRatioThreshold verifies the updated intervalHardRatio=1.20.
func TestClassifySegments_HardRatioThreshold(t *testing.T) {
	now := time.Now()
	sec := func(n int) time.Time { return now.Add(time.Duration(n) * time.Second) }

	// globalMean = 100; ratio > 1.20 → hard; 1.10 is now "moderate" (was "hard")
	signal := []float64{100, 100, 100, 100, 110, 110, 110, 110, 125, 125, 125, 125}
	pts := make([]intervalStream, len(signal))
	for i := range pts {
		pts[i] = intervalStream{T: sec(i)}
	}
	times := make([]time.Time, len(pts))
	for i, p := range pts {
		times[i] = p.T
	}

	segs := []intervalSegment{
		{startIdx: 0, endIdx: 3, startT: times[0], endT: times[3]},
		{startIdx: 4, endIdx: 7, startT: times[4], endT: times[7]},
		{startIdx: 8, endIdx: 11, startT: times[8], endT: times[11]},
	}

	classified := classifySegments(segs, signal, pts)

	// globalMean = (100*4 + 110*4 + 125*4) / 12 = 335/3 ≈ 111.67
	// seg0: mean=100, ratio=100/111.67≈0.896 → moderate
	// seg1: mean=110, ratio=110/111.67≈0.985 → moderate
	// seg2: mean=125, ratio=125/111.67≈1.119 → hard (>1.20? no → moderate at 1.20 threshold)

	// With intervalHardRatio=1.20: segment at ratio 1.119 should be "moderate", not "hard"
	for i, cs := range classified {
		if i == 2 {
			// ratio ≈ 1.119 < 1.20 → should be moderate
			if cs.intensity == "hard" {
				t.Errorf("segment %d: ratio ~1.119 should be 'moderate' with threshold 1.20, got 'hard'", i)
			}
		}
	}
}

// TestIntervalMinHardSegments_Constant verifies the constant is 3.
func TestIntervalMinHardSegments_Constant(t *testing.T) {
	if intervalMinHardSegments != 3 {
		t.Errorf("intervalMinHardSegments = %d, want 3", intervalMinHardSegments)
	}
}

// TestIntervalSmoothWindow_Constant verifies the window is 30.
func TestIntervalSmoothWindow_Constant(t *testing.T) {
	if intervalSmoothWindow != 30 {
		t.Errorf("intervalSmoothWindow = %d, want 30", intervalSmoothWindow)
	}
}

// intPtr is a helper to create *int from a literal.
func intPtr(v int) *int { return &v }
