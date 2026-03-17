package activityutil

import (
	"math"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func fptr(v float64) *float64 { return &v }

// TestFillGaps_AllNilDistance — classic pre-2015 Garmin: no distance at all.
// Should derive cumulative distance from GPS.
func TestFillGaps_AllNilDistance(t *testing.T) {
	base := time.Date(2014, 6, 21, 14, 0, 0, 0, time.UTC)
	lat1, lng1 := 48.35, 10.90
	lat2, lng2 := 48.36, 10.91 // ~1.3 km away
	lat3, lng3 := 48.37, 10.92

	points := []model.StreamPoint{
		{Time: base, Lat: fptr(lat1), Lng: fptr(lng1)},
		{Time: base.Add(60 * time.Second), Lat: fptr(lat2), Lng: fptr(lng2)},
		{Time: base.Add(120 * time.Second), Lat: fptr(lat3), Lng: fptr(lng3)},
	}

	got := FillMissingDistanceAndSpeed(points)

	// First point should be 0
	if got[0].DistanceM == nil || *got[0].DistanceM != 0 {
		t.Errorf("point 0: want distance 0, got %v", got[0].DistanceM)
	}
	// Subsequent points should have increasing distance
	for i := 1; i < len(got); i++ {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil", i)
		}
		if *got[i].DistanceM <= *got[i-1].DistanceM {
			t.Errorf("point %d: distance not increasing: %f <= %f", i, *got[i].DistanceM, *got[i-1].DistanceM)
		}
	}
	// All should have speed
	for i := 1; i < len(got); i++ {
		if got[i].SpeedMS == nil {
			t.Errorf("point %d: SpeedMS is nil", i)
		}
	}
}

// TestFillGaps_AllHaveDistance — modern device: all points have distance.
// Should be a no-op.
func TestFillGaps_AllHaveDistance(t *testing.T) {
	base := time.Now()
	points := []model.StreamPoint{
		{Time: base, DistanceM: fptr(0), SpeedMS: fptr(5.0), Lat: fptr(48.0), Lng: fptr(10.0)},
		{Time: base.Add(time.Second), DistanceM: fptr(500), SpeedMS: fptr(5.0), Lat: fptr(48.01), Lng: fptr(10.01)},
		{Time: base.Add(2 * time.Second), DistanceM: fptr(1000), SpeedMS: fptr(5.0), Lat: fptr(48.02), Lng: fptr(10.02)},
	}

	got := FillMissingDistanceAndSpeed(points)

	// All original values preserved
	for i, p := range got {
		if *p.DistanceM != *points[i].DistanceM {
			t.Errorf("point %d: distance changed from %f to %f", i, *points[i].DistanceM, *p.DistanceM)
		}
	}
}

// TestFillGaps_MiddleGap — Szenario D: first and last sessions have distance,
// middle session does not. The gap should be filled with Haversine-derived values
// that bridge seamlessly from the last real value.
func TestFillGaps_MiddleGap(t *testing.T) {
	base := time.Now()
	lat, lng := 48.35, 10.90

	points := []model.StreamPoint{
		// Session 1: has distance
		{Time: base, DistanceM: fptr(0), Lat: fptr(lat), Lng: fptr(lng)},
		{Time: base.Add(10 * time.Second), DistanceM: fptr(100), Lat: fptr(lat + 0.001), Lng: fptr(lng)},
		// Session 2: NO distance (10h gap)
		{Time: base.Add(10 * time.Hour), Lat: fptr(lat + 0.01), Lng: fptr(lng + 0.01)},
		{Time: base.Add(10*time.Hour + 10*time.Second), Lat: fptr(lat + 0.02), Lng: fptr(lng + 0.01)},
		// Session 3: has distance again (resets from 0, but fixDistanceResets already ran)
		{Time: base.Add(17 * time.Hour), DistanceM: fptr(5000), Lat: fptr(lat + 0.05), Lng: fptr(lng + 0.02)},
		{Time: base.Add(17*time.Hour + 10*time.Second), DistanceM: fptr(5100), Lat: fptr(lat + 0.06), Lng: fptr(lng + 0.02)},
	}

	got := FillMissingDistanceAndSpeed(points)

	// Points 0,1 keep their real values
	if *got[0].DistanceM != 0 {
		t.Errorf("point 0: want 0, got %f", *got[0].DistanceM)
	}
	if *got[1].DistanceM != 100 {
		t.Errorf("point 1: want 100, got %f", *got[1].DistanceM)
	}
	// Points 2,3 should be filled — derived from last known (100m) + haversine
	if got[2].DistanceM == nil {
		t.Fatal("point 2: DistanceM should be filled")
	}
	if *got[2].DistanceM <= 100 {
		t.Errorf("point 2: should be > 100 (last real value), got %f", *got[2].DistanceM)
	}
	if got[3].DistanceM == nil {
		t.Fatal("point 3: DistanceM should be filled")
	}
	if *got[3].DistanceM <= *got[2].DistanceM {
		t.Errorf("point 3: should be > point 2")
	}
	// Points 4,5 keep their real values (5000, 5100) — NOT overwritten
	if *got[4].DistanceM != 5000 {
		t.Errorf("point 4: want 5000, got %f", *got[4].DistanceM)
	}
	if *got[5].DistanceM != 5100 {
		t.Errorf("point 5: want 5100, got %f", *got[5].DistanceM)
	}
}

// TestFillGaps_NoGPS — indoor points: no lat/lng, no distance. Should stay nil.
func TestFillGaps_NoGPS(t *testing.T) {
	base := time.Now()
	points := []model.StreamPoint{
		{Time: base},
		{Time: base.Add(time.Second)},
	}
	got := FillMissingDistanceAndSpeed(points)
	for i, p := range got {
		if p.DistanceM != nil {
			t.Errorf("point %d: DistanceM should be nil for indoor", i)
		}
	}
}

// TestFillGaps_SpeedNotDerivedAcrossLargeGap — speed should NOT be derived
// across gaps > 5min (these are pauses, not travel).
func TestFillGaps_SpeedNotDerivedAcrossLargeGap(t *testing.T) {
	base := time.Now()
	points := []model.StreamPoint{
		{Time: base, Lat: fptr(48.35), Lng: fptr(10.90)},
		{Time: base.Add(10 * time.Hour), Lat: fptr(48.36), Lng: fptr(10.91)}, // 10h gap
	}
	got := FillMissingDistanceAndSpeed(points)
	// Distance SHOULD be derived (cumulative is always useful)
	if got[0].DistanceM == nil || got[1].DistanceM == nil {
		t.Error("distance should be derived even across large gap")
	}
	// Speed should NOT be derived across 10h gap (would be nonsensically slow)
	if got[1].SpeedMS != nil {
		t.Error("speed should not be derived across >5min gap")
	}
}

// TestFillGaps_MonotonicityWhenGPSOvershootsReal — GPS-derived cumDist can
// exceed the next real value (Haversine overestimates over gaps). The function
// must enforce monotonicity by adjusting the real value upward.
func TestFillGaps_MonotonicityWhenGPSOvershootsReal(t *testing.T) {
	base := time.Now()
	// Real: 500m, then 3 nil points where GPS adds ~700m, then real=500.1m (after fixDistanceResets).
	// Without monotonicity guard: cumDist goes 500 → ~1200 → 500.1 (backwards jump!)
	points := []model.StreamPoint{
		{Time: base, DistanceM: fptr(500), Lat: fptr(48.35), Lng: fptr(10.90)},
		// Gap with GPS (nil distance) — large Haversine jump
		{Time: base.Add(10 * time.Second), Lat: fptr(48.36), Lng: fptr(10.91)},  // ~1.3km away
		{Time: base.Add(20 * time.Second), Lat: fptr(48.37), Lng: fptr(10.92)},  // another ~1.3km
		// Real value that is LESS than cumDist (fixDistanceResets output)
		{Time: base.Add(30 * time.Second), DistanceM: fptr(500.1), Lat: fptr(48.38), Lng: fptr(10.93)},
		{Time: base.Add(40 * time.Second), DistanceM: fptr(800), Lat: fptr(48.39), Lng: fptr(10.94)},
	}

	got := FillMissingDistanceAndSpeed(points)

	// Verify strict monotonicity — no backwards jumps
	var prev float64
	for i, p := range got {
		if p.DistanceM == nil {
			continue
		}
		if *p.DistanceM < prev {
			t.Errorf("point %d: distance decreased from %f to %f (monotonicity violation)", i, prev, *p.DistanceM)
		}
		prev = *p.DistanceM
	}

	// Point 3 should have been adjusted upward (not 500.1)
	if got[3].DistanceM != nil && *got[3].DistanceM < *got[2].DistanceM {
		t.Errorf("point 3: should be >= point 2 (%f), got %f", *got[2].DistanceM, *got[3].DistanceM)
	}
}

// TestFillGaps_FirstPointNilThenReal — first points have nil distance,
// then real values appear. GPS-derived values for the start should seamlessly
// connect to the first real value.
func TestFillGaps_FirstPointNilThenReal(t *testing.T) {
	base := time.Now()
	points := []model.StreamPoint{
		{Time: base, Lat: fptr(48.35), Lng: fptr(10.90)},                                                  // nil dist
		{Time: base.Add(10 * time.Second), Lat: fptr(48.3501), Lng: fptr(10.9001)},                        // nil dist
		{Time: base.Add(20 * time.Second), DistanceM: fptr(500), Lat: fptr(48.3502), Lng: fptr(10.9002)}, // real
	}

	got := FillMissingDistanceAndSpeed(points)

	// Points 0,1 should have derived values
	if got[0].DistanceM == nil {
		t.Fatal("point 0: should have derived distance")
	}
	if got[1].DistanceM == nil {
		t.Fatal("point 1: should have derived distance")
	}
	// Point 2 keeps 500 (or is adjusted up if GPS derived more)
	if got[2].DistanceM == nil || *got[2].DistanceM < 500 {
		t.Errorf("point 2: should be >= 500, got %v", got[2].DistanceM)
	}
	// Monotonicity
	if *got[1].DistanceM < *got[0].DistanceM {
		t.Error("not monotonic between points 0 and 1")
	}
	if *got[2].DistanceM < *got[1].DistanceM {
		t.Error("not monotonic between points 1 and 2")
	}
}

// ── FixDistanceResets tests ──────────────────────────────────────────────────

// TestFixDistanceResets_SingleLap — no change for normal progression
func TestFixDistanceResets_Shared_SingleLap(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: fptr(0.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(200.0)},
		{Time: now.Add(2 * time.Second), DistanceM: fptr(500.0)},
		{Time: now.Add(3 * time.Second), DistanceM: fptr(800.0)},
	}
	got := FixDistanceResets(points)
	want := []float64{0.0, 200.0, 500.0, 800.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if *got[i].DistanceM != w {
			t.Errorf("point %d: got %f, want %f", i, *got[i].DistanceM, w)
		}
	}
}

// TestFixDistanceResets_TwoLapReset — second lap resets distance to 0
func TestFixDistanceResets_Shared_TwoLapReset(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: fptr(200.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(500.0)},
		// GOTOES Reset: dist drops near 0
		{Time: now.Add(2 * time.Second), DistanceM: fptr(0.12)},
		{Time: now.Add(3 * time.Second), DistanceM: fptr(300.0)},
		{Time: now.Add(4 * time.Second), DistanceM: fptr(700.0)},
	}
	got := FixDistanceResets(points)
	want := []float64{200.0, 500.0, 500.12, 800.0, 1200.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if math.Abs(*got[i].DistanceM-w) > 0.01 {
			t.Errorf("point %d: got %f, want %f", i, *got[i].DistanceM, w)
		}
	}
}

// TestFixDistanceResets_NilDistances — nil points stay nil
func TestFixDistanceResets_Shared_NilDistances(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: fptr(200.0)},
		{Time: now.Add(1 * time.Second), DistanceM: nil},
		{Time: now.Add(2 * time.Second), DistanceM: fptr(0.5)}, // Reset after 200m
		{Time: now.Add(3 * time.Second), DistanceM: fptr(300.0)},
	}
	got := FixDistanceResets(points)
	if got[1].DistanceM != nil {
		t.Error("point 1: DistanceM should remain nil")
	}
	if got[2].DistanceM == nil {
		t.Fatal("point 2: DistanceM is nil")
	}
	if math.Abs(*got[2].DistanceM-200.5) > 0.01 {
		t.Errorf("point 2: got %f, want 200.5", *got[2].DistanceM)
	}
}

// TestFixDistanceResets_SmallDrop — small backward jump (<100m) is not a reset
func TestFixDistanceResets_Shared_SmallDrop(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: fptr(500.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(498.0)},
		{Time: now.Add(2 * time.Second), DistanceM: fptr(600.0)},
	}
	got := FixDistanceResets(points)
	if math.Abs(*got[1].DistanceM-498.0) > 0.01 {
		t.Errorf("point 1: got %f, want 498.0", *got[1].DistanceM)
	}
	if math.Abs(*got[2].DistanceM-600.0) > 0.01 {
		t.Errorf("point 2: got %f, want 600.0", *got[2].DistanceM)
	}
}

// TestFixDistanceResets_ThreeSegments — three merged segments (triple reset)
func TestFixDistanceResets_Shared_ThreeSegments(t *testing.T) {
	now := time.Now()
	points := []model.StreamPoint{
		{Time: now, DistanceM: fptr(0.0)},
		{Time: now.Add(1 * time.Second), DistanceM: fptr(500.0)},
		{Time: now.Add(2 * time.Second), DistanceM: fptr(0.1)},
		{Time: now.Add(3 * time.Second), DistanceM: fptr(400.0)},
		{Time: now.Add(4 * time.Second), DistanceM: fptr(0.2)},
		{Time: now.Add(5 * time.Second), DistanceM: fptr(300.0)},
	}
	got := FixDistanceResets(points)
	want := []float64{0.0, 500.0, 500.1, 900.0, 900.2, 1200.0}
	for i, w := range want {
		if got[i].DistanceM == nil {
			t.Fatalf("point %d: DistanceM is nil, want %f", i, w)
		}
		if math.Abs(*got[i].DistanceM-w) > 0.01 {
			t.Errorf("point %d: got %f, want %f", i, *got[i].DistanceM, w)
		}
	}
}

// ── CalcRecordingTime tests ─────────────────────────────────────────────────

// TestCalcRecordingTime_Continuous — no gaps, recording time ≈ elapsed
func TestCalcRecordingTime_Continuous(t *testing.T) {
	base := time.Now()
	// Simulate 60 minutes of recording with 1-minute intervals (all < 5min)
	var points []model.StreamPoint
	for i := 0; i <= 60; i++ {
		points = append(points, model.StreamPoint{Time: base.Add(time.Duration(i) * time.Minute)})
	}
	got := CalcRecordingTime(points)
	if got != 3600 { // 60 min
		t.Errorf("want 3600s, got %d", got)
	}
}

// TestCalcRecordingTime_MeringScenario — 3 clusters with multi-hour gaps.
// Only the intervals within clusters should count.
func TestCalcRecordingTime_MeringScenario(t *testing.T) {
	base := time.Date(2014, 6, 21, 14, 19, 22, 0, time.UTC)
	// Build clusters with 1-minute intervals within each cluster
	var points []model.StreamPoint
	// Cluster 1: 51 minutes (51 intervals × 60s = 3060s)
	for i := 0; i <= 51; i++ {
		points = append(points, model.StreamPoint{Time: base.Add(time.Duration(i) * time.Minute)})
	}
	// 10.4h gap → Cluster 2: 15 minutes (900s)
	gap1 := base.Add(10*time.Hour + 24*time.Minute)
	for i := 0; i <= 15; i++ {
		points = append(points, model.StreamPoint{Time: gap1.Add(time.Duration(i) * time.Minute)})
	}
	// 5.1h gap → Cluster 3: 37 minutes (2220s)
	gap2 := gap1.Add(5*time.Hour + 45*time.Minute)
	for i := 0; i <= 37; i++ {
		points = append(points, model.StreamPoint{Time: gap2.Add(time.Duration(i) * time.Minute)})
	}
	got := CalcRecordingTime(points)
	// 3060 + 900 + 2220 = 6180s
	if got != 6180 {
		t.Errorf("want 6180s, got %d", got)
	}
}

// TestCalcRecordingTime_NoGaps — all intervals < threshold → full elapsed
func TestCalcRecordingTime_NoGaps(t *testing.T) {
	base := time.Now()
	points := []model.StreamPoint{
		{Time: base},
		{Time: base.Add(2 * time.Minute)},
		{Time: base.Add(4 * time.Minute)},
		{Time: base.Add(6 * time.Minute)},
	}
	got := CalcRecordingTime(points)
	if got != 360 { // 6 min
		t.Errorf("want 360s, got %d", got)
	}
}

// TestCalcRecordingTime_TooFew — <2 points → 0
func TestCalcRecordingTime_TooFew(t *testing.T) {
	got := CalcRecordingTime([]model.StreamPoint{{Time: time.Now()}})
	if got != 0 {
		t.Errorf("want 0, got %d", got)
	}
}

