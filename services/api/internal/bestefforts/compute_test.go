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
	for i := 0; i < n; i++ {
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
func f64(v float64) *float64 { return &v }
func i16(v int16) *int16     { return &v }

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
	for i := 0; i < 4000; i++ {
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
	for i := 0; i < n; i++ {
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
	for i := 0; i < n; i++ {
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
	result := ComputePowerBestEffort(pts, cat)
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
	for i := 0; i < n; i++ {
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
	result := ComputePowerBestEffort(pts, cat)
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

// ── Elevation tests ───────────────────────────────────────────────────────────

// TestComputeElevationBestEffort_Basic: ascending ramp → find 100 m gain.
func TestComputeElevationBestEffort_Basic(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// 500 pts, 1 m/s ascent rate, 1 Hz → each step +1 m altitude.
	n := 500
	pts := make([]model.StreamPoint, n)
	for i := 0; i < n; i++ {
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
	for i := 0; i < n; i++ {
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
	cat := Category{Key: "total_distance", Type: TypeValueSuperlative}
	r := ComputeSuperlative(cat, 42195.0, 500.0, 300.0, 14400.0)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if math.Abs(r.ValueNumeric-42195.0) > 0.01 {
		t.Errorf("ValueNumeric = %.2f, want 42195.0", r.ValueNumeric)
	}

	cat2 := Category{Key: "total_elevation", Type: TypeValueSuperlative}
	r2 := ComputeSuperlative(cat2, 42195.0, 500.0, 300.0, 14400.0)
	if math.Abs(r2.ValueNumeric-500.0) > 0.01 {
		t.Errorf("ValueNumeric = %.2f, want 500.0", r2.ValueNumeric)
	}
}
