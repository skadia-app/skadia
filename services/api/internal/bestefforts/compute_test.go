package bestefforts

import (
	"math"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// makeStream creates a synthetic stream of n points at the given speed.
// Points are spaced 1 second apart starting at t0.
// DistanceM accumulates linearly, AltitudeM is flat (100 m), PowerW is nil.
func makeStream(n int, speedMs float64) []model.StreamPoint {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := make([]model.StreamPoint, n)
	for i := range n {
		dist := speedMs * float64(i)
		alt := 100.0
		pts[i] = model.StreamPoint{
			Time:      t0.Add(time.Duration(i) * time.Second),
			DistanceM: &dist,
			AltitudeM: &alt,
		}
	}
	return pts
}

// ptr helpers
//
//go:fix inline
func f64(v float64) *float64 { return new(v) }

//go:fix inline
func i16(v int16) *int16 { return new(v) }

// ── Distance tests ────────────────────────────────────────────────────────────

// TestComputeDistanceBestEffort_Basic: 2000 pts at 3 m/s → 5 km best effort.
// At 3 m/s it takes 5000/3 ≈ 1666.7 s → elapsed = 1666 s (integer truncation).
func TestComputeDistanceBestEffort_Basic(t *testing.T) {
	stream := makeStream(2000, 3.0)
	cat := Category{Key: "5km", Type: TypeTimeLower, DistanceM: 5000}
	result := ComputeDistanceBestEffort(stream, cat)
	if result == nil {
		t.Fatal("expected non-nil result for 5 km in 2000-pt stream at 3 m/s")
	}
	// 5000 m / 3 m/s = 1666.666… s  →  window of 1667 points spans 1666 s
	expected := 1666
	if math.Abs(float64(result.ElapsedTimeS-expected)) > 2 {
		t.Errorf("ElapsedTimeS = %d, want ~%d", result.ElapsedTimeS, expected)
	}
	if result.Category != "5km" {
		t.Errorf("Category = %q, want %q", result.Category, "5km")
	}
}

// TestComputeDistanceBestEffort_TooShort: stream shorter than target → nil.
func TestComputeDistanceBestEffort_TooShort(t *testing.T) {
	// 100 pts at 3 m/s → total 297 m, target 5 km
	stream := makeStream(100, 3.0)
	cat := Category{Key: "5km", Type: TypeTimeLower, DistanceM: 5000}
	result := ComputeDistanceBestEffort(stream, cat)
	if result != nil {
		t.Errorf("expected nil for stream shorter than target, got elapsed=%d", result.ElapsedTimeS)
	}
}

// TestComputeDistanceBestEffort_GPSJump: a large jump mid-stream does not poison
// the result; a valid segment after the jump is still found.
func TestComputeDistanceBestEffort_GPSJump(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Build a 4000-point stream at 3 m/s with a 500 m GPS jump at index 1000.
	// First segment: indices 0–999 → dist 0–2997 m (2997 m, not enough for 3 km target).
	// Second segment: indices 1000–3999 → dist = 3*i+500, range 3500–12497 m (9000 m window).
	// A 3 km best effort must be found entirely within the second segment.
	pts := make([]model.StreamPoint, 4000)
	for i := range 4000 {
		dist := 3.0 * float64(i)
		if i >= 1000 {
			dist += 500.0 // forward jump of 500 m at index 1000
		}
		alt := 100.0
		pts[i] = model.StreamPoint{
			Time:      t0.Add(time.Duration(i) * time.Second),
			DistanceM: &dist,
			AltitudeM: &alt,
		}
	}

	cat := Category{Key: "3km", Type: TypeTimeLower, DistanceM: 3000}
	result := ComputeDistanceBestEffort(pts, cat)
	// The first segment covers only 0–2997 m (< 3 km target), so the best 3 km
	// effort must come entirely from the second segment (indices >= 1000).
	if result == nil {
		t.Fatal("expected non-nil result despite GPS jump")
	}
	// Both endpoints must lie within the post-jump segment.
	if result.StreamStartIdx < 1000 {
		t.Errorf("result starts before jump: start=%d end=%d", result.StreamStartIdx, result.StreamEndIdx)
	}
}

// ── Power tests ───────────────────────────────────────────────────────────────

// makePowerStream builds a stream of n points spaced 1 s apart, all at powerW watts.
func makePowerStream(n int, powerW int16) []model.StreamPoint {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := make([]model.StreamPoint, n)
	for i := range n {
		p := powerW
		pts[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i) * time.Second),
			PowerW: &p,
		}
	}
	return pts
}

// TestComputePowerBestEffort_5min: 10-min stream; middle 5 min at 300 W, rest at 100 W.
// Best 5-min mean must equal 300 W.
func TestComputePowerBestEffort_5min(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	n := 600 // 10 minutes, 1 Hz
	pts := make([]model.StreamPoint, n)
	for i := range n {
		var pw int16 = 100
		// Seconds 150–449 (inclusive) → 300 seconds of 300 W in the middle.
		if i >= 150 && i < 450 {
			pw = 300
		}
		p := pw
		pts[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i) * time.Second),
			PowerW: &p,
		}
	}

	cat := Category{Key: "5min_power", Type: TypeValueHigher, DurationS: 300}
	result := ComputePowerBestEffort(pts, cat, nil)
	if result == nil {
		t.Fatal("expected non-nil result for 5-min power in 10-min stream")
	}
	// The best 5-min window that contains the 300 W block entirely gives mean 300 W.
	if math.Abs(result.ValueNumeric-300.0) > 1.0 {
		t.Errorf("ValueNumeric = %.2f, want ~300.0", result.ValueNumeric)
	}
}

// TestComputePowerBestEffort_NilIsZero: first 60 s at 200 W, rest nil (coasting).
// Best 1-min mean must be 200 W (window entirely in the 200 W segment).
func TestComputePowerBestEffort_NilIsZero(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	n := 300 // 5 minutes total
	pts := make([]model.StreamPoint, n)
	for i := range n {
		pt := model.StreamPoint{
			Time: t0.Add(time.Duration(i) * time.Second),
		}
		if i < 60 {
			pw := int16(200)
			pt.PowerW = &pw
		}
		// i >= 60 → PowerW remains nil (coasting → treated as 0)
		pts[i] = pt
	}

	cat := Category{Key: "1min_power", Type: TypeValueHigher, DurationS: 60}
	result := ComputePowerBestEffort(pts, cat, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Best window is the first 61 points (0–60), mean = sum(200*60 + 0) / 61 ≈ 196.7 W.
	// Or window 0–59 (60 points, 59 s < 60 s target) — the algorithm needs exactly 60 s.
	// Window [0..60] is 60 s with 60×200 + 1×0 = 12000 W·s / 61 pts ≈ 196.7 W.
	// Either way, it must be clearly better than the nil-only windows (~0 W).
	if result.ValueNumeric < 150.0 {
		t.Errorf("ValueNumeric = %.2f, expected >=150 (best window should prefer 200W segment)", result.ValueNumeric)
	}
}

// TestComputePowerBestEffort_VariableSampling: stream with 5 s Smart Recording
// intervals. At uniform 200 W the time-weighted mean must equal 200 W.
// A second window with 1 s resolution at 300 W must be preferred over the
// 200 W window, demonstrating that time-weighting does not distort selection.
func TestComputePowerBestEffort_VariableSampling(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Build a stream: first 240 s at 5 s intervals (48 samples) at 200 W,
	// then 60 s at 1 s intervals (60 samples) at 300 W.
	// Target: 60 s best power. Expected winner: 300 W window.
	pts := make([]model.StreamPoint, 0, 108)
	ts := t0
	for range 48 {
		pw := int16(200)
		pts = append(pts, model.StreamPoint{Time: ts, PowerW: &pw})
		ts = ts.Add(5 * time.Second)
	}
	for range 60 {
		pw := int16(300)
		pts = append(pts, model.StreamPoint{Time: ts, PowerW: &pw})
		ts = ts.Add(1 * time.Second)
	}

	cat := Category{Key: "1min_power", Type: TypeValueHigher, DurationS: 60}
	result := ComputePowerBestEffort(pts, cat, nil)
	if result == nil {
		t.Fatal("expected non-nil result for variable-sampling stream")
	}
	// The 300 W window must win.
	if result.ValueNumeric < 250.0 {
		t.Errorf("ValueNumeric = %.2f, want ~300 (300W 1Hz window should beat 200W 5s window)", result.ValueNumeric)
	}

	// Verify time-weighted mean: a pure 5 s stream at constant 200 W
	// must yield exactly 200 W (not distorted by sample count).
	pts5s := make([]model.StreamPoint, 0, 24)
	ts2 := t0
	for range 24 {
		pw := int16(200)
		pts5s = append(pts5s, model.StreamPoint{Time: ts2, PowerW: &pw})
		ts2 = ts2.Add(5 * time.Second)
	}
	cat2 := Category{Key: "1min_power", Type: TypeValueHigher, DurationS: 60}
	result2 := ComputePowerBestEffort(pts5s, cat2, nil)
	if result2 == nil {
		t.Fatal("expected non-nil result for 5 s stream")
	}
	if math.Abs(result2.ValueNumeric-200.0) > 1.0 {
		t.Errorf("time-weighted mean = %.2f, want 200.0 for uniform 5s/200W stream", result2.ValueNumeric)
	}
}

// ── Elevation tests ───────────────────────────────────────────────────────────

// TestComputeElevationBestEffort_Basic: ascending ramp → find 100 m gain.
func TestComputeElevationBestEffort_Basic(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 500 pts, 1 m/s ascent rate, 1 Hz → each step +1 m altitude.
	n := 500
	pts := make([]model.StreamPoint, n)
	for i := range n {
		alt := float64(i) // 0, 1, 2, … 499 m
		pts[i] = model.StreamPoint{
			Time:      t0.Add(time.Duration(i) * time.Second),
			AltitudeM: &alt,
		}
	}
	cat := Category{Key: "100m_climb", Type: TypeTimeLower, ElevationM: 100}
	result := ComputeElevationBestEffort(pts, cat, nil)
	if result == nil {
		t.Fatal("expected non-nil result for 100 m gain in 500-pt ramp")
	}
	// 100 m gain takes 100 s on a 1 m/s ramp.
	if math.Abs(float64(result.ElapsedTimeS-100)) > 2 {
		t.Errorf("ElapsedTimeS = %d, want ~100", result.ElapsedTimeS)
	}
}

// TestComputeElevationBestEffort_TooShort: stream with <100 m total gain → nil.
func TestComputeElevationBestEffort_TooShort(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 50 pts, +1 m each → 49 m total gain
	n := 50
	pts := make([]model.StreamPoint, n)
	for i := range n {
		alt := float64(i)
		pts[i] = model.StreamPoint{Time: t0.Add(time.Duration(i) * time.Second), AltitudeM: &alt}
	}
	cat := Category{Key: "100m_climb", Type: TypeTimeLower, ElevationM: 100}
	result := ComputeElevationBestEffort(pts, cat, nil)
	if result != nil {
		t.Errorf("expected nil for insufficient elevation gain, got elapsed=%d", result.ElapsedTimeS)
	}
}

// TestComputeSuperlative_Basic: values are routed to the correct field.
func TestComputeSuperlative_Basic(t *testing.T) {
	cat := Category{Key: "sup_longest_distance", Type: TypeValueSuperlative}
	r := ComputeSuperlative(cat, 42195.0, 500.0, 300.0, 14400.0)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if math.Abs(r.ValueNumeric-42195.0) > 0.01 {
		t.Errorf("ValueNumeric = %.2f, want 42195.0", r.ValueNumeric)
	}

	cat2 := Category{Key: "sup_most_elevation", Type: TypeValueSuperlative}
	r2 := ComputeSuperlative(cat2, 42195.0, 500.0, 300.0, 14400.0)
	if math.Abs(r2.ValueNumeric-500.0) > 0.01 {
		t.Errorf("ValueNumeric = %.2f, want 500.0", r2.ValueNumeric)
	}
}

// TestComputeSuperlative_ZeroGuard: val<=0 must return nil.
func TestComputeSuperlative_ZeroGuard(t *testing.T) {
	cat := Category{Key: "sup_most_elevation", Type: TypeValueSuperlative}
	r := ComputeSuperlative(cat, 10000.0, 0.0, 300.0, 3600.0)
	if r != nil {
		t.Errorf("expected nil for zero elevGainM, got ValueNumeric=%.2f", r.ValueNumeric)
	}
}

// ── Stuck-value heuristic tests ───────────────────────────────────────────────

// makePowerStreamSparse builds a stream where each point is spaced intervalS
// seconds apart, all at the given power. Useful for Smart Recording simulations.
func makePowerStreamSparse(n int, powerW int16, intervalS int) []model.StreamPoint {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := make([]model.StreamPoint, n)
	for i := range n {
		p := powerW
		pts[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i*intervalS) * time.Second),
			PowerW: &p,
		}
	}
	return pts
}

// TestFindStuckPowerRegions_Detected: 150 points at 701W with 1Hz timestamps,
// median 250W → the 150s run (>120s) at >2×250=500W must be detected.
func TestFindStuckPowerRegions_Detected(t *testing.T) {
	// Build a stream: 100 points at 250W followed by 150 points at 701W.
	// Median of mixed stream: sorted 250*100 then 701*150; median at pos 125 = 701W.
	// But we pass medianPower=250 directly to keep the test deterministic.
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := make([]model.StreamPoint, 250)
	for i := range 250 {
		var pw int16
		if i < 100 {
			pw = 250
		} else {
			pw = 701
		}
		p := pw
		pts[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i) * time.Second),
			PowerW: &p,
		}
	}

	regions := findStuckPowerRegions(pts, 250.0, 120.0, 2.0)
	if len(regions) != 1 {
		t.Fatalf("expected 1 stuck region, got %d", len(regions))
	}
	if regions[0].StartIdx != 100 || regions[0].EndIdx != 249 {
		t.Errorf("region = [%d,%d], want [100,249]", regions[0].StartIdx, regions[0].EndIdx)
	}
}

// TestFindStuckPowerRegions_TooShort: 30s identical at 500W must not be flagged
// because it is below the 120s minimum duration.
func TestFindStuckPowerRegions_TooShort(t *testing.T) {
	pts := makePowerStream(30, 500)
	regions := findStuckPowerRegions(pts, 200.0, 120.0, 2.0)
	if len(regions) != 0 {
		t.Errorf("expected no stuck regions for 30s run, got %d", len(regions))
	}
}

// TestFindStuckPowerRegions_LowValue: 250W for 200s with median 240W → not flagged
// because 250 < 2 × 240 = 480.
func TestFindStuckPowerRegions_LowValue(t *testing.T) {
	pts := makePowerStream(200, 250)
	regions := findStuckPowerRegions(pts, 240.0, 120.0, 2.0)
	if len(regions) != 0 {
		t.Errorf("expected no stuck regions for below-threshold power, got %d", len(regions))
	}
}

// TestFindStuckPowerRegions_ZeroWatts: 0W for 200s must not be flagged (coasting).
func TestFindStuckPowerRegions_ZeroWatts(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := make([]model.StreamPoint, 200)
	for i := range 200 {
		var zero int16 = 0
		pts[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i) * time.Second),
			PowerW: &zero,
		}
	}
	regions := findStuckPowerRegions(pts, 200.0, 120.0, 2.0)
	if len(regions) != 0 {
		t.Errorf("expected no stuck regions for 0W coasting, got %d", len(regions))
	}
}

// TestFindStuckPowerRegions_SmartRecording: 30 points at 5s intervals = 150s at 701W.
// Must be detected using time-based duration (not sample count).
func TestFindStuckPowerRegions_SmartRecording(t *testing.T) {
	pts := makePowerStreamSparse(30, 701, 5)
	// 30 points × 5s = 150s at 701W; median = 200W so threshold = 400W.
	regions := findStuckPowerRegions(pts, 200.0, 120.0, 2.0)
	if len(regions) != 1 {
		t.Fatalf("expected 1 stuck region for Smart Recording stream, got %d", len(regions))
	}
}

// TestIsInStuckRegion: overlap, no overlap, partial overlap.
func TestIsInStuckRegion(t *testing.T) {
	regions := []StuckRegion{{StartIdx: 50, EndIdx: 100}}

	// Fully inside
	if !isInStuckRegion(60, 90, regions) {
		t.Error("expected overlap: window [60,90] inside [50,100]")
	}
	// Fully before
	if isInStuckRegion(10, 40, regions) {
		t.Error("expected no overlap: window [10,40] before [50,100]")
	}
	// Fully after
	if isInStuckRegion(110, 150, regions) {
		t.Error("expected no overlap: window [110,150] after [50,100]")
	}
	// Partial overlap left
	if !isInStuckRegion(30, 60, regions) {
		t.Error("expected overlap: window [30,60] partial left of [50,100]")
	}
	// Partial overlap right
	if !isInStuckRegion(90, 120, regions) {
		t.Error("expected overlap: window [90,120] partial right of [50,100]")
	}
	// No regions → never overlaps
	if isInStuckRegion(0, 200, nil) {
		t.Error("expected no overlap with nil regions")
	}
}

// TestComputePowerBestEffort_SuspiciousFlag: stream with a stuck region that
// covers the best power window must set Suspicious=true on the result.
func TestComputePowerBestEffort_SuspiciousFlag(t *testing.T) {
	// 600-point stream: first 300s at 200W, then 300s at 800W (stuck).
	// The 5-min best power window will land in the 800W stuck region.
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := make([]model.StreamPoint, 600)
	for i := range 600 {
		var pw int16
		if i < 300 {
			pw = 200
		} else {
			pw = 800
		}
		p := pw
		pts[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i) * time.Second),
			PowerW: &p,
		}
	}

	medianPower := medianPowerFromStream(pts) // median of equal halves 200+800 → 500W
	stuckRegions := findStuckPowerRegions(pts, medianPower, 120.0, 2.0)
	// 800 > 2 × 500 = 1000? No, 800 < 1000. Let us use a lower median.
	// Recalculate manually: 300 pts at 200W, 300 pts at 800W → sorted list:
	//   positions 0-299 → 200, positions 300-599 → 800.
	//   n=600, mid=300, even: (vals[299]+vals[300])/2 = (200+800)/2 = 500.
	// threshold = 2 × 500 = 1000, but 800 < 1000 → no stuck region with default params.
	// Use threshold multiplier = 1.5 for this test to force detection.
	stuckRegions = findStuckPowerRegions(pts, medianPower, 120.0, 1.5)
	// 800 > 1.5 × 500 = 750 → yes, stuck region detected.

	cat := Category{Key: "5min_power", Type: TypeValueHigher, DurationS: 300}
	result := ComputePowerBestEffort(pts, cat, stuckRegions)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.Suspicious {
		t.Errorf("expected Suspicious=true for best window inside stuck region, got false (start=%d end=%d regions=%v)",
			result.StreamStartIdx, result.StreamEndIdx, stuckRegions)
	}
}

// TestComputePowerBestEffort_NotSuspicious: best window outside any stuck region
// must leave Suspicious=false.
func TestComputePowerBestEffort_NotSuspicious(t *testing.T) {
	// Stuck region at end of stream, best power window clearly before it.
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := make([]model.StreamPoint, 600)
	for i := range 600 {
		var pw int16
		if i < 300 {
			pw = 400 // the good segment
		} else {
			pw = 800 // stuck — but best window is in the 400W zone
		}
		p := pw
		pts[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i) * time.Second),
			PowerW: &p,
		}
	}

	// Force a stuck region over [300,599] at 800W > 2×400=800? Exactly at boundary.
	// Use median = 350W so threshold = 700W < 800W → detected.
	stuckRegions := findStuckPowerRegions(pts, 350.0, 120.0, 2.0)

	// Best window for 5min at 400W (indices 0-299) vs 800W (indices 300-599).
	// 800 > 400, so best window is actually in the stuck region.
	// For NotSuspicious test, use a range that places the best effort before the stuck.
	// Override: build a stream where 400W segment is first and there's no stuck there.
	stuckRegions = []StuckRegion{{StartIdx: 400, EndIdx: 599}}

	cat := Category{Key: "1min_power", Type: TypeValueHigher, DurationS: 60}
	result := ComputePowerBestEffort(pts, cat, stuckRegions)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Best 1-min is in the 800W zone (indices 300-599), which overlaps [400,599].
	// So actually this will be suspicious. Adjust the test: use a stream where
	// the best window is clearly before the stuck region.
	pts2 := make([]model.StreamPoint, 600)
	for i := range 600 {
		var pw int16
		if i < 60 {
			pw = 500 // best 1-min window
		} else {
			pw = 100
		}
		p := pw
		pts2[i] = model.StreamPoint{
			Time:   t0.Add(time.Duration(i) * time.Second),
			PowerW: &p,
		}
	}
	stuckRegions2 := []StuckRegion{{StartIdx: 300, EndIdx: 599}}
	result2 := ComputePowerBestEffort(pts2, cat, stuckRegions2)
	if result2 == nil {
		t.Fatal("expected non-nil result for pts2")
	}
	if result2.Suspicious {
		t.Errorf("expected Suspicious=false for best window [%d,%d] outside stuck region [300,599]",
			result2.StreamStartIdx, result2.StreamEndIdx)
	}
}

// TestMedianPowerFromStream: verify median computation for odd and even lengths.
func TestMedianPowerFromStream(t *testing.T) {
	// Odd: [100, 200, 300] → median 200
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	makePt := func(i int, pw int16) model.StreamPoint {
		p := pw
		return model.StreamPoint{Time: t0.Add(time.Duration(i) * time.Second), PowerW: &p}
	}
	pts := []model.StreamPoint{makePt(0, 100), makePt(1, 200), makePt(2, 300)}
	if m := medianPowerFromStream(pts); math.Abs(m-200.0) > 0.01 {
		t.Errorf("median = %.2f, want 200.0", m)
	}

	// Even: [100, 200, 300, 400] → median (200+300)/2 = 250
	pts2 := []model.StreamPoint{makePt(0, 100), makePt(1, 200), makePt(2, 300), makePt(3, 400)}
	if m := medianPowerFromStream(pts2); math.Abs(m-250.0) > 0.01 {
		t.Errorf("median = %.2f, want 250.0", m)
	}

	// No power data → 0
	pts3 := []model.StreamPoint{{Time: t0}}
	if m := medianPowerFromStream(pts3); m != 0 {
		t.Errorf("median = %.2f, want 0 for nil power", m)
	}
}
