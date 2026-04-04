package runningform

import (
	"math"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// makeRunPoints creates n StreamPoints spaced 1 second apart with the given
// constant GCT and cadence values.
func makeRunPoints(n int, gct int16, cad int16) []model.StreamPoint {
	t0 := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	points := make([]model.StreamPoint, n)
	for i := range n {
		g, c := gct, cad
		points[i] = model.StreamPoint{
			Time:                t0.Add(time.Duration(i) * time.Second),
			GroundContactTimeMs: &g,
			Cadence:             &c,
		}
	}
	return points
}

// TestComputeDutyFactor verifies the happy-path Tier-1 result.
//
// GCT = 250 ms, Cadence = 180 spm
// stride_duration = 60000/180 = 333.33 ms
// duty_factor = 250/333.33 ≈ 0.75
func TestComputeDutyFactor(t *testing.T) {
	points := makeRunPoints(1000, 250, 180)
	res := Compute(points)
	if res == nil {
		t.Fatal("expected non-nil result for 1000 points")
	}
	if res.Tier < 1 {
		t.Errorf("expected Tier >= 1, got %d", res.Tier)
	}

	const want = 250.0 / (60000.0 / 180.0) // ≈ 0.75
	if math.Abs(res.DutyFactor-want) > 1e-9 {
		t.Errorf("DutyFactor: want %.6f, got %.6f", want, res.DutyFactor)
	}
}

// TestComputeVerticalRatio verifies Tier-2 metrics.
//
// VO = 80 mm, StepLength = 1200 mm → VR = 80/1200 ≈ 0.06667
func TestComputeVerticalRatio(t *testing.T) {
	points := makeRunPoints(1000, 250, 180)

	vo := int16(80)
	sl := 1200.0
	for i := range points {
		points[i].VerticalOscillationMm = &vo
		points[i].StepLengthMm = &sl
	}

	res := Compute(points)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.Tier < 2 {
		t.Errorf("expected Tier >= 2, got %d", res.Tier)
	}

	const want = 80.0 / 1200.0
	if math.Abs(res.VerticalRatio-want) > 1e-9 {
		t.Errorf("VerticalRatio: want %.6f, got %.6f", want, res.VerticalRatio)
	}
}

// TestComputeTooFewPoints verifies that fewer than minPointsPerQuartile
// duty-factor samples return nil.
func TestComputeTooFewPoints(t *testing.T) {
	points := makeRunPoints(30, 250, 180)
	// 30 points spans 29 seconds — well under the 15-minute minimum — but we
	// want to test the sample-count guard specifically.  Spread the points over
	// 16 minutes so only the sample-count guard fires.
	t0 := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	for i := range points {
		g, c := int16(250), int16(180)
		points[i] = model.StreamPoint{
			Time:                t0.Add(time.Duration(i) * 32 * time.Second), // 32s spacing → 16 min total
			GroundContactTimeMs: &g,
			Cadence:             &c,
		}
	}

	res := Compute(points)
	if res != nil {
		t.Errorf("expected nil for only 30 duty-factor samples, got Tier %d", res.Tier)
	}
}

// TestComputeTooShortDuration verifies that runs shorter than 15 minutes
// return nil even when there are many data points.
func TestComputeTooShortDuration(t *testing.T) {
	// 100 points at 6-second spacing = 99 × 6 s = 594 s ≈ 9.9 min (< 15 min).
	t0 := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	points := make([]model.StreamPoint, 100)
	for i := range points {
		g, c := int16(250), int16(180)
		points[i] = model.StreamPoint{
			Time:                t0.Add(time.Duration(i) * 6 * time.Second),
			GroundContactTimeMs: &g,
			Cadence:             &c,
		}
	}

	res := Compute(points)
	if res != nil {
		t.Errorf("expected nil for 10-min run, got Tier %d", res.Tier)
	}
}

// TestFatigueIndex verifies that FatigueDuty is positive when GCT increases
// over the second half of an activity (indicating fatigue).
//
// First 900 points: GCT = 240 ms → DF ≈ 0.72
// Last 300 points:  GCT = 270 ms → DF ≈ 0.81
// Expected: FatigueDuty > 0
func TestFatigueIndex(t *testing.T) {
	const n = 1200
	t0 := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	points := make([]model.StreamPoint, n)

	for i := range points {
		var gct int16
		if i < 900 {
			gct = 240
		} else {
			gct = 270
		}
		c := int16(180)
		points[i] = model.StreamPoint{
			Time:                t0.Add(time.Duration(i) * time.Second),
			GroundContactTimeMs: &gct,
			Cadence:             &c,
		}
	}

	res := Compute(points)
	if res == nil {
		t.Fatal("expected non-nil result for 1200 points")
	}
	if res.FatigueDuty <= 0 {
		t.Errorf("expected positive FatigueDuty (fatigue), got %.4f", res.FatigueDuty)
	}
}
