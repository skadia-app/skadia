package activityutil

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ptrI16(v int16) *int16 { return &v }

func mkPointWithAlt(lat, lng, alt float64, sec int) model.StreamPoint {
	return model.StreamPoint{
		Time:      t0Plus(sec),
		Lat:       ptr(lat),
		Lng:       ptr(lng),
		AltitudeM: ptr(alt),
	}
}

func mkPointWithAltCadence(lat, lng, alt float64, cadence int16, sec int) model.StreamPoint {
	sp := mkPointWithAlt(lat, lng, alt, sec)
	sp.Cadence = ptrI16(cadence)
	return sp
}

// --- Test 1: Flat spike caught ---
func TestCorrectSpeedOutliers_FlatSpike(t *testing.T) {
	// Cycling on flat. Points ~33m apart at 1s = 120 km/h.
	// Base max = 70 km/h * 1.3 overshoot = 91 km/h trigger. 120 > 91 → corrected.
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 500, 0),
		mkPointWithAlt(48.00005, 11.00000, 500, 1),
		mkPointWithAlt(48.00010, 11.00000, 500, 2),
		mkPointWithAlt(48.00040, 11.00000, 500, 3), // ~33m/1s = 120 km/h
		mkPointWithAlt(48.00045, 11.00000, 500, 4),
		mkPointWithAlt(48.00050, 11.00000, 500, 5),
		mkPointWithAlt(48.00055, 11.00000, 500, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	result, report := CorrectSpeedOutliers(points, "cycling", cfg)

	if report.CorrectedCount == 0 {
		t.Fatal("expected correction for 120 km/h on flat cycling")
	}
	if result[3].RawLat == nil {
		t.Fatal("expected RawLat on corrected point")
	}
}

// --- Test 2: Downhill not flagged ---
func TestCorrectSpeedOutliers_DownhillNotFlagged(t *testing.T) {
	// Cycling at 80 km/h (~22 m/s) downhill. Gradient ~ -13%.
	// Max = 70 + 5*13 = 135 km/h. Trigger = 135*1.3 = 175 km/h. 80 << 175 → not flagged.
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 600, 0),
		mkPointWithAlt(48.00005, 11.00000, 599, 1),
		mkPointWithAlt(48.00010, 11.00000, 598, 2),
		mkPointWithAlt(48.00030, 11.00000, 595, 3),
		mkPointWithAlt(48.00050, 11.00000, 592, 4),
		mkPointWithAlt(48.00070, 11.00000, 589, 5),
		mkPointWithAlt(48.00090, 11.00000, 586, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "cycling", cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("80 km/h downhill cycling should NOT be corrected, got %d", report.CorrectedCount)
	}
}

// --- Test 3: Gradient bonus is the deciding factor ---
func TestCorrectSpeedOutliers_GradientBonusSavesPoint(t *testing.T) {
	// Hiking at 50 km/h (~13.9 m/s) on -15% gradient (steep trail descent).
	// Without gradient: base = 8.3 m/s (30 km/h). Trigger = 8.3*1.3 = 10.8 m/s (39 km/h).
	//   13.9 > 10.8 → WOULD be flagged.
	// With gradient: max = 8.3 + 1.4*15 = 29.3 m/s (105 km/h). Trigger = 29.3*1.3 = 38.1.
	//   13.9 < 38.1 → NOT flagged. Gradient bonus is the reason.
	//
	// Geometry: ~13.9m horizontal per point, -2.1m altitude per point = -15% gradient.
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 600, 0),
		mkPointWithAlt(48.00002, 11.00000, 599.7, 1),  // ~2.2 m/s warmup
		mkPointWithAlt(48.00004, 11.00000, 599.4, 2),
		mkPointWithAlt(48.00016, 11.00000, 597.3, 3),   // ~13.3m in 1s = 48 km/h, drop 2.1m = -15.8%
		mkPointWithAlt(48.00018, 11.00000, 597.0, 4),
		mkPointWithAlt(48.00020, 11.00000, 596.7, 5),
		mkPointWithAlt(48.00022, 11.00000, 596.4, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "hiking", cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("48 km/h hiking on -15%% descent should NOT be corrected, got %d", report.CorrectedCount)
	}
}

// --- Test 4: Cadence protection ---
func TestCorrectSpeedOutliers_CadenceProtection(t *testing.T) {
	// High speed on flat but cadence > 0.
	// Base = 70 km/h. With cadence boost 1.3: 91 km/h. Trigger = 91*1.3 = 118 km/h.
	// Speed ~95 km/h should NOT be flagged with cadence.
	points := []model.StreamPoint{
		mkPointWithAltCadence(48.00000, 11.00000, 500, 80, 0),
		mkPointWithAltCadence(48.00005, 11.00000, 500, 80, 1),
		mkPointWithAltCadence(48.00010, 11.00000, 500, 82, 2),
		mkPointWithAltCadence(48.00035, 11.00000, 500, 85, 3), // ~28m = 100 km/h + cadence
		mkPointWithAltCadence(48.00040, 11.00000, 500, 80, 4),
		mkPointWithAltCadence(48.00045, 11.00000, 500, 78, 5),
		mkPointWithAltCadence(48.00050, 11.00000, 500, 75, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "cycling", cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("100 km/h with cadence should NOT be corrected, got %d", report.CorrectedCount)
	}
}

// --- Test 5: Gondola protection (consecutive) ---
func TestCorrectSpeedOutliers_GondolaNotFlagged(t *testing.T) {
	// Gondola: consistent ~48 km/h (13.3 m/s) while hiking. Base foot = 30 km/h.
	// Trigger = 30*1.3 = 39 km/h. 48 > 39 → all points are candidates.
	// But 8+ consecutive points → consecutive filter skips them all.
	points := []model.StreamPoint{
		mkPointWithAlt(47.00000, 11.00000, 800, 0),
		mkPointWithAlt(47.00012, 11.00000, 810, 1),  // ~13.3 m/s = 48 km/h
		mkPointWithAlt(47.00024, 11.00000, 820, 2),
		mkPointWithAlt(47.00036, 11.00000, 830, 3),
		mkPointWithAlt(47.00048, 11.00000, 840, 4),
		mkPointWithAlt(47.00060, 11.00000, 850, 5),
		mkPointWithAlt(47.00072, 11.00000, 860, 6),
		mkPointWithAlt(47.00084, 11.00000, 870, 7),
		mkPointWithAlt(47.00096, 11.00000, 880, 8),
		mkPointWithAlt(47.00108, 11.00000, 890, 9),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "hiking", cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("gondola should NOT be corrected, got %d", report.CorrectedCount)
	}
	// Verify the consecutive filter was actually exercised (not vacuous)
	if report.CandidateCount == 0 {
		t.Fatal("expected candidates for gondola points — test is vacuous")
	}
	if report.SkippedConsec == 0 {
		t.Fatal("expected SkippedConsec > 0 — consecutive filter not exercised")
	}
}

func copyPoints(pts []model.StreamPoint) []model.StreamPoint {
	c := make([]model.StreamPoint, len(pts))
	copy(c, pts)
	return c
}

// --- Test 6: Walking mild spike ---

func TestCorrectSpeedOutliers_WalkingMildSpike(t *testing.T) {
	// Walking at ~5 km/h, one point at ~45 km/h (~12.5 m/s) on flat.
	// Base foot = 30 km/h. Trigger = 30*1.3 = 39 km/h. 45 > 39 → corrected.
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 500, 0),
		mkPointWithAlt(48.000014, 11.00000, 500, 1),
		mkPointWithAlt(48.000028, 11.00000, 500, 2),
		mkPointWithAlt(48.000140, 11.00000, 500, 3), // ~12.5 m/s = 45 km/h
		mkPointWithAlt(48.000042, 11.00000, 500, 4),
		mkPointWithAlt(48.000056, 11.00000, 500, 5),
		mkPointWithAlt(48.000070, 11.00000, 500, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "hiking", cfg)

	if report.CorrectedCount == 0 {
		t.Fatal("45 km/h walking on flat should be corrected")
	}
}

// --- Test 7: No altitude data ---
func TestCorrectSpeedOutliers_NoAltitude(t *testing.T) {
	// No altitude → gradient unknown → use flat base only.
	// 120 km/h cycling without altitude → trigger 91 km/h → corrected.
	points := []model.StreamPoint{
		mkPoint(48.00000, 11.00000, 0),
		mkPoint(48.00005, 11.00000, 1),
		mkPoint(48.00010, 11.00000, 2),
		mkPoint(48.00040, 11.00000, 3), // ~33m = 120 km/h
		mkPoint(48.00015, 11.00000, 4),
		mkPoint(48.00020, 11.00000, 5),
		mkPoint(48.00025, 11.00000, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "cycling", cfg)

	if report.CorrectedCount == 0 {
		t.Fatal("120 km/h cycling without altitude should be corrected")
	}
}

// --- Test 8: Ski Alpin fast ---
func TestCorrectSpeedOutliers_SkiAlpinFast(t *testing.T) {
	// Ski at 100 km/h downhill. Base = 80 km/h.
	// At -20% gradient: max = 80 + 5*20 = 180. Trigger = 180*1.3 = 234. 100 << 234.
	points := []model.StreamPoint{
		mkPointWithAlt(47.00000, 11.00000, 2000, 0),
		mkPointWithAlt(47.00010, 11.00000, 1998, 1),
		mkPointWithAlt(47.00020, 11.00000, 1996, 2),
		mkPointWithAlt(47.00045, 11.00000, 1991, 3), // ~28 m/s = 100 km/h
		mkPointWithAlt(47.00070, 11.00000, 1986, 4),
		mkPointWithAlt(47.00095, 11.00000, 1981, 5),
		mkPointWithAlt(47.00120, 11.00000, 1976, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "alpine_skiing", cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("100 km/h alpine skiing downhill should NOT be corrected, got %d", report.CorrectedCount)
	}
}

// --- Test 9: Short shuttle at 80 km/h — protected by MinOvershootRatio ---
func TestCorrectSpeedOutliers_ShortShuttleNotFlagged(t *testing.T) {
	// MTB shuttle: 3 consecutive points at 80 km/h (~22.2 m/s) on flat.
	// Base bike = 19.4 m/s (70 km/h). Speed 22.2 > 19.4 → exceeds base.
	// But trigger = 19.4 * 1.3 = 25.2 m/s (91 km/h). 22.2 < 25.2 → NOT flagged.
	// This verifies MinOvershootRatio protects shuttles between base and trigger.
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 500, 0),
		mkPointWithAlt(48.00005, 11.00000, 500, 1),
		mkPointWithAlt(48.00025, 11.00000, 500, 2), // ~22.2 m/s = 80 km/h
		mkPointWithAlt(48.00045, 11.00000, 500, 3), // ~80 km/h
		mkPointWithAlt(48.00065, 11.00000, 500, 4), // ~80 km/h
		mkPointWithAlt(48.00070, 11.00000, 500, 5),
		mkPointWithAlt(48.00075, 11.00000, 500, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	_, report := CorrectSpeedOutliers(points, "cycling", cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("80 km/h shuttle should NOT be corrected (below trigger 91 km/h), got %d", report.CorrectedCount)
	}
}

// --- Test 10: Cascading neighbor protection ---
func TestCorrectSpeedOutliers_NeighborNotDoubleCorrected(t *testing.T) {
	// When point idx is corrected (interpolated), the speed at idx+1 changes
	// because it was measured from the displaced position. The algorithm must
	// remove idx+1 from candidates after correcting idx to prevent cascading.
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 500, 0),
		mkPointWithAlt(48.00005, 11.00000, 500, 1),
		mkPointWithAlt(48.00040, 11.00000, 500, 2), // spike: ~39m = 140 km/h
		mkPointWithAlt(48.00015, 11.00000, 500, 3), // THIS should NOT also be corrected
		mkPointWithAlt(48.00020, 11.00000, 500, 4),
		mkPointWithAlt(48.00025, 11.00000, 500, 5),
		mkPointWithAlt(48.00030, 11.00000, 500, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	result, report := CorrectSpeedOutliers(points, "cycling", cfg)

	if report.CorrectedCount != 1 {
		t.Fatalf("expected exactly 1 correction (the spike), got %d", report.CorrectedCount)
	}
	// Point 3 should NOT have RawLat set
	if result[3].RawLat != nil {
		t.Fatal("point after spike should NOT be corrected (cascading protection)")
	}
}

// --- Test 11: Already corrected by spatial filter ---
func TestCorrectSpeedOutliers_AlreadyCorrectedBySpaceFilter(t *testing.T) {
	// Point already has RawLat set by spatial filter. Speed filter should
	// still correct the position but NOT overwrite RawLat.
	origLat := 48.00040
	origLng := 11.00000
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 500, 0),
		mkPointWithAlt(48.00005, 11.00000, 500, 1),
		{
			Time: t0Plus(2), Lat: ptr(48.00040), Lng: ptr(11.00000),
			AltitudeM: ptr(500.0),
			RawLat: &origLat, RawLng: &origLng, // already set by spatial filter
		},
		mkPointWithAlt(48.00015, 11.00000, 500, 3),
		mkPointWithAlt(48.00020, 11.00000, 500, 4),
		mkPointWithAlt(48.00025, 11.00000, 500, 5),
		mkPointWithAlt(48.00030, 11.00000, 500, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	result, _ := CorrectSpeedOutliers(points, "cycling", cfg)

	// RawLat should still be the ORIGINAL value from the spatial filter
	if result[2].RawLat == nil || *result[2].RawLat != origLat {
		t.Fatal("RawLat should preserve the spatial filter's original, not overwrite")
	}
}

// --- Test 12: Report deterministic ---
func TestCorrectSpeedOutliers_ReportDeterministic(t *testing.T) {
	points := []model.StreamPoint{
		mkPointWithAlt(48.00000, 11.00000, 500, 0),
		mkPointWithAlt(48.00005, 11.00000, 500, 1),
		mkPointWithAlt(48.00040, 11.00000, 500, 2), // spike
		mkPointWithAlt(48.00010, 11.00000, 500, 3),
		mkPointWithAlt(48.00015, 11.00000, 500, 4),
		mkPointWithAlt(48.00020, 11.00000, 500, 5),
		mkPointWithAlt(48.00025, 11.00000, 500, 6),
	}
	cfg := DefaultSpeedCorrectionConfig()
	var firstReport SpeedCorrectionReport
	for i := range 10 {
		_, report := CorrectSpeedOutliers(copyPoints(points), "cycling", cfg)
		if i == 0 {
			firstReport = report
		} else if len(report.Points) != len(firstReport.Points) {
			t.Fatalf("run %d: report length changed", i)
		}
	}
}
