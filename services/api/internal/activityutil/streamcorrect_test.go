package activityutil

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ptr(f float64) *float64 { return &f }

func t0Plus(seconds int) time.Time {
	return time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)
}

func mkPoint(lat, lng float64, sec int) model.StreamPoint {
	return model.StreamPoint{Time: t0Plus(sec), Lat: ptr(lat), Lng: ptr(lng)}
}

// TestCorrectGPSOutliers_SingleSpike — straight line with one point jumping
// ~550m north. Expect exactly 1 correction with RawLat storing the original.
func TestCorrectGPSOutliers_SingleSpike(t *testing.T) {
	points := []model.StreamPoint{
		mkPoint(48.0000, 11.0000, 0),
		mkPoint(48.0010, 11.0000, 1),
		mkPoint(48.0020, 11.0000, 2),
		mkPoint(48.0080, 11.0000, 3), // spike: ~550m north
		mkPoint(48.0040, 11.0000, 4),
		mkPoint(48.0050, 11.0000, 5),
		mkPoint(48.0060, 11.0000, 6),
	}

	cfg := DefaultCorrectionConfig()
	result, report := CorrectGPSOutliers(points, cfg)

	if report.CorrectedCount != 1 {
		t.Fatalf("expected 1 correction, got %d", report.CorrectedCount)
	}

	corrected := result[3]
	if corrected.RawLat == nil || corrected.RawLng == nil {
		t.Fatal("corrected point should have RawLat/RawLng set")
	}
	if *corrected.RawLat != 48.0080 {
		t.Errorf("RawLat should store original 48.0080, got %f", *corrected.RawLat)
	}

	// Interpolated lat should be midpoint of neighbors (48.0020 + 48.0040) / 2 = 48.0030
	expectedLat := (48.0020 + 48.0040) / 2
	if diff := *corrected.Lat - expectedLat; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("interpolated lat expected %f, got %f", expectedLat, *corrected.Lat)
	}
}

// TestCorrectGPSOutliers_SwitchbackNotFlagged — gentle U-turn with no isolated
// spike. Expect 0 corrections.
func TestCorrectGPSOutliers_SwitchbackNotFlagged(t *testing.T) {
	// Gentle U-turn: go north, curve east, come back south
	points := []model.StreamPoint{
		mkPoint(48.0000, 11.0000, 0),
		mkPoint(48.0005, 11.0000, 1),
		mkPoint(48.0010, 11.0002, 2),
		mkPoint(48.0010, 11.0005, 3),
		mkPoint(48.0005, 11.0007, 4),
		mkPoint(48.0000, 11.0007, 5),
	}

	cfg := DefaultCorrectionConfig()
	_, report := CorrectGPSOutliers(points, cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("expected 0 corrections for switchback, got %d", report.CorrectedCount)
	}
}

// TestCorrectGPSOutliers_TightHairpinNotFlagged — sharp alpine switchback with
// points ~100m apart. Expect 0 corrections.
func TestCorrectGPSOutliers_TightHairpinNotFlagged(t *testing.T) {
	points := []model.StreamPoint{
		mkPoint(47.00000, 11.00000, 0),
		mkPoint(47.00090, 11.00000, 1),
		mkPoint(47.00180, 11.00000, 2),
		mkPoint(47.00190, 11.00020, 3), // hairpin turn
		mkPoint(47.00100, 11.00020, 4),
		mkPoint(47.00010, 11.00020, 5),
		mkPoint(47.00000, 11.00040, 6),
	}

	cfg := DefaultCorrectionConfig()
	_, report := CorrectGPSOutliers(points, cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("expected 0 corrections for hairpin, got %d", report.CorrectedCount)
	}
}

// TestCorrectGPSOutliers_NoGPS — points without Lat/Lng. Expect 0 corrections
// and all points preserved.
func TestCorrectGPSOutliers_NoGPS(t *testing.T) {
	points := []model.StreamPoint{
		{Time: t0Plus(0)},
		{Time: t0Plus(1)},
		{Time: t0Plus(2)},
		{Time: t0Plus(3)},
		{Time: t0Plus(4)},
		{Time: t0Plus(5)},
	}

	cfg := DefaultCorrectionConfig()
	result, report := CorrectGPSOutliers(points, cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("expected 0 corrections without GPS, got %d", report.CorrectedCount)
	}
	if len(result) != len(points) {
		t.Fatalf("expected %d points preserved, got %d", len(points), len(result))
	}
}

// TestCorrectGPSOutliers_ConsecutiveBouncesSkipped — 6 alternating spikes produce
// 4 consecutive candidate indices in the interior. With MaxConsecutiveSpikes=3,
// the run of 4 exceeds the limit and the entire run is skipped.
func TestCorrectGPSOutliers_ConsecutiveBouncesSkipped(t *testing.T) {
	// Zigzag: 6 spikes alternating +1km / -1km from baseline. The 2 edge
	// spikes (indices 2, 7) have low detour and are not candidates, but the
	// 4 inner spikes (indices 3–6) all have extreme detour with isolation ≈ 1.
	points := []model.StreamPoint{
		mkPoint(48.0000, 11.0000, 0),  // [0] normal
		mkPoint(48.0010, 11.0000, 1),  // [1] normal
		mkPoint(48.0110, 11.0000, 2),  // [2] spike edge (low detour — not candidate)
		mkPoint(47.9910, 11.0000, 3),  // [3] SPIKE candidate
		mkPoint(48.0110, 11.0000, 4),  // [4] SPIKE candidate
		mkPoint(47.9910, 11.0000, 5),  // [5] SPIKE candidate
		mkPoint(48.0110, 11.0000, 6),  // [6] SPIKE candidate
		mkPoint(47.9910, 11.0000, 7),  // [7] spike edge (low detour — not candidate)
		mkPoint(48.0020, 11.0000, 8),  // [8] normal
		mkPoint(48.0030, 11.0000, 9),  // [9] normal
	}

	cfg := DefaultCorrectionConfig()
	cfg.MaxConsecutiveSpikes = 3
	cfg.MinIsolation = 0.5 // lower so candidates pass isolation filter
	_, report := CorrectGPSOutliers(points, cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("expected 0 corrections for consecutive bounces, got %d", report.CorrectedCount)
	}
	// Verify the test isn't vacuous — candidates were found and skipped.
	if report.SkippedConsec == 0 && report.CandidateCount == 0 {
		t.Fatal("test is vacuous: no candidates or skipped-consec found")
	}
}

// TestCorrectGPSOutliers_GondolaNotFlagged — consistent movement in one
// direction (~10m/s). Expect 0 corrections.
func TestCorrectGPSOutliers_GondolaNotFlagged(t *testing.T) {
	// Straight line heading north, ~110m between points (1s intervals)
	points := []model.StreamPoint{
		mkPoint(48.0000, 11.0000, 0),
		mkPoint(48.0010, 11.0000, 1),
		mkPoint(48.0020, 11.0000, 2),
		mkPoint(48.0030, 11.0000, 3),
		mkPoint(48.0040, 11.0000, 4),
		mkPoint(48.0050, 11.0000, 5),
		mkPoint(48.0060, 11.0000, 6),
	}

	cfg := DefaultCorrectionConfig()
	_, report := CorrectGPSOutliers(points, cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("expected 0 corrections for gondola, got %d", report.CorrectedCount)
	}
}

// TestCorrectGPSOutliers_MicroDriftIgnored — standing still with ~2m GPS drift.
// High detour is possible but distance below MinDistanceM (5m). Expect 0.
func TestCorrectGPSOutliers_MicroDriftIgnored(t *testing.T) {
	// Cluster of points within ~2m of each other
	base := 48.00000
	points := []model.StreamPoint{
		mkPoint(base, 11.00000, 0),
		mkPoint(base+0.000001, 11.00000, 1),
		mkPoint(base-0.000001, 11.000001, 2),
		mkPoint(base+0.000002, 11.00000, 3),
		mkPoint(base-0.000001, 11.000001, 4),
		mkPoint(base, 11.00000, 5),
	}

	cfg := DefaultCorrectionConfig()
	_, report := CorrectGPSOutliers(points, cfg)

	if report.CorrectedCount != 0 {
		t.Fatalf("expected 0 corrections for micro drift, got %d", report.CorrectedCount)
	}
}

// TestCorrectGPSOutliers_ReportDeterministic — running the same correction 10
// times with fresh copies must produce identical report point order.
func TestCorrectGPSOutliers_ReportDeterministic(t *testing.T) {
	makePoints := func() []model.StreamPoint {
		return []model.StreamPoint{
			mkPoint(48.0000, 11.0000, 0),
			mkPoint(48.0010, 11.0000, 1),
			mkPoint(48.0020, 11.0000, 2),
			mkPoint(48.0080, 11.0000, 3), // spike
			mkPoint(48.0040, 11.0000, 4),
			mkPoint(48.0050, 11.0000, 5),
			mkPoint(48.0060, 11.0000, 6),
		}
	}

	cfg := DefaultCorrectionConfig()

	// Get reference report.
	refPoints := makePoints()
	_, refReport := CorrectGPSOutliers(refPoints, cfg)

	if refReport.CorrectedCount == 0 {
		t.Fatal("reference run should produce at least 1 correction")
	}

	// Run 10 more times and compare.
	for run := 0; run < 10; run++ {
		pts := makePoints()
		_, rep := CorrectGPSOutliers(pts, cfg)

		if len(rep.Points) != len(refReport.Points) {
			t.Fatalf("run %d: point count %d != reference %d", run, len(rep.Points), len(refReport.Points))
		}
		for i := range rep.Points {
			if rep.Points[i].Index != refReport.Points[i].Index {
				t.Fatalf("run %d: point[%d].Index=%d != reference %d",
					run, i, rep.Points[i].Index, refReport.Points[i].Index)
			}
			if rep.Points[i].Corrected != refReport.Points[i].Corrected {
				t.Fatalf("run %d: point[%d].Corrected=%v != reference %v",
					run, i, rep.Points[i].Corrected, refReport.Points[i].Corrected)
			}
		}
	}
}
