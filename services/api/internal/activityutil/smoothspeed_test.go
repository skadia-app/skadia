package activityutil

import (
	"math"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func f64(v float64) *float64 { return &v }

func mkSpeedPoint(speedMS float64, sec int) model.StreamPoint {
	return model.StreamPoint{
		Time:    time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC).Add(time.Duration(sec) * time.Second),
		SpeedMS: f64(speedMS),
	}
}

func TestSmoothSpeed_SpikeEliminated(t *testing.T) {
	// Median of [5, 5, 120, 5, 5] = 5. The spike is completely removed.
	points := []model.StreamPoint{
		mkSpeedPoint(5, 0),
		mkSpeedPoint(5, 1),
		mkSpeedPoint(120, 2), // spike
		mkSpeedPoint(5, 3),
		mkSpeedPoint(5, 4),
	}
	result := SmoothSpeed(points, 2) // 5-point window
	if math.Abs(*result[2].SpeedMS-5) > 0.01 {
		t.Fatalf("spike should be eliminated: got %f, want 5.0", *result[2].SpeedMS)
	}
}

func TestSmoothSpeed_NormalDataPreserved(t *testing.T) {
	// Monotonically increasing: median should track the trend.
	points := []model.StreamPoint{
		mkSpeedPoint(10, 0),
		mkSpeedPoint(12, 1),
		mkSpeedPoint(14, 2),
		mkSpeedPoint(16, 3),
		mkSpeedPoint(18, 4),
	}
	result := SmoothSpeed(points, 1) // 3-point window
	// Middle point: median of [12, 14, 16] = 14 (unchanged)
	if math.Abs(*result[2].SpeedMS-14) > 0.01 {
		t.Fatalf("normal data should be preserved: got %f, want 14", *result[2].SpeedMS)
	}
}

func TestSmoothSpeed_GapBreaksWindow(t *testing.T) {
	// 6-minute gap between points 2 and 3 should prevent them from being in the same window.
	points := []model.StreamPoint{
		mkSpeedPoint(5, 0),
		mkSpeedPoint(5, 1),
		mkSpeedPoint(100, 2),   // spike
		mkSpeedPoint(5, 362),   // 6 minutes later — gap > 300s
		mkSpeedPoint(5, 363),
	}
	result := SmoothSpeed(points, 2)
	// Point 2: right neighbors are across a gap, window only extends left.
	// Window = [5, 5, 100] → median = 5. Spike eliminated.
	if *result[2].SpeedMS > 10 {
		t.Fatalf("spike with gap should still be smoothed by left neighbors: got %f", *result[2].SpeedMS)
	}
	// Point 3: left neighbors are across a gap, only right neighbor available.
	// Window = [5, 5] → median = 5. Original preserved.
	if math.Abs(*result[3].SpeedMS-5) > 0.01 {
		t.Fatalf("point after gap should be ~5: got %f", *result[3].SpeedMS)
	}
}

func TestSmoothSpeed_NilSpeedsSkipped(t *testing.T) {
	points := []model.StreamPoint{
		{Time: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{Time: time.Date(2025, 1, 1, 10, 0, 1, 0, time.UTC)},
		mkSpeedPoint(10, 2),
		{Time: time.Date(2025, 1, 1, 10, 0, 3, 0, time.UTC)},
	}
	result := SmoothSpeed(points, 1)
	if result[0].SpeedMS != nil {
		t.Fatal("nil speed should remain nil")
	}
	if result[1].SpeedMS != nil {
		t.Fatal("nil speed should remain nil")
	}
	if result[2].SpeedMS == nil {
		t.Fatal("non-nil speed should remain")
	}
}

func TestSmoothSpeed_WindowLargerThanData(t *testing.T) {
	// Window = 3 (7-point), but only 3 data points. Should not panic.
	points := []model.StreamPoint{
		mkSpeedPoint(10, 0),
		mkSpeedPoint(20, 1),
		mkSpeedPoint(30, 2),
	}
	result := SmoothSpeed(points, 3)
	// Median of all 3: 20
	if math.Abs(*result[1].SpeedMS-20) > 0.01 {
		t.Fatalf("got %f, want 20", *result[1].SpeedMS)
	}
}

func TestSmoothSpeed_SinglePointWindow(t *testing.T) {
	// Window = 0 should return data unchanged.
	points := []model.StreamPoint{
		mkSpeedPoint(100, 0),
		mkSpeedPoint(5, 1),
	}
	result := SmoothSpeed(points, 0)
	if *result[0].SpeedMS != 100 {
		t.Fatal("window=0 should not smooth")
	}
}

func TestSmoothSpeed_EdgePoints(t *testing.T) {
	// First and last points have smaller effective windows.
	points := []model.StreamPoint{
		mkSpeedPoint(100, 0), // edge: window = [100, 5, 5] → median = 5
		mkSpeedPoint(5, 1),
		mkSpeedPoint(5, 2),
		mkSpeedPoint(5, 3),
		mkSpeedPoint(100, 4), // edge: window = [5, 5, 100] → median = 5
	}
	result := SmoothSpeed(points, 2)
	if *result[0].SpeedMS > 10 {
		t.Fatalf("first point spike should be smoothed: got %f", *result[0].SpeedMS)
	}
	if *result[4].SpeedMS > 10 {
		t.Fatalf("last point spike should be smoothed: got %f", *result[4].SpeedMS)
	}
}
