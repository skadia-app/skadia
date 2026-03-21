package enrichment

import (
	"math"
	"testing"
)

func TestSmoothElevation_Flat(t *testing.T) {
	// Flat data -> output identical (within tolerance)
	alt := []float64{500, 500, 500, 500, 500, 500, 500, 500, 500, 500}
	dist := []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90}
	result := SmoothElevation(alt, dist)
	if len(result) != len(alt) {
		t.Fatalf("length mismatch: got %d, want %d", len(result), len(alt))
	}
	for i, v := range result {
		if math.Abs(v-500) > 0.5 {
			t.Errorf("point %d: got %.1f, want ~500", i, v)
		}
	}
}

func TestSmoothElevation_NoisyClimb(t *testing.T) {
	// Linear climb 500->600m over 500m with +/-5m GPS noise
	// After smoothing the trend should be preserved, noise reduced
	n := 100
	alt := make([]float64, n)
	dist := make([]float64, n)
	for i := range n {
		dist[i] = float64(i) * 5.0 // 5m steps
		alt[i] = 500.0 + float64(i)*1.0
		// Simulate GPS noise
		if i%3 == 0 {
			alt[i] += 3.0
		} else if i%3 == 1 {
			alt[i] -= 3.0
		}
	}
	result := SmoothElevation(alt, dist)

	// Trend check: start ~500, end ~600
	if result[0] < 495 || result[0] > 510 {
		t.Errorf("start: got %.1f, want ~500", result[0])
	}
	if result[n-1] < 590 || result[n-1] > 610 {
		t.Errorf("end: got %.1f, want ~600", result[n-1])
	}

	// Noise should be reduced: max point-to-point delta < 3m (instead of +/-6m raw)
	maxDelta := 0.0
	for i := 1; i < len(result); i++ {
		d := math.Abs(result[i] - result[i-1])
		if d > maxDelta {
			maxDelta = d
		}
	}
	if maxDelta > 3.0 {
		t.Errorf("max delta: got %.1f, want < 3.0 (noise should be smoothed)", maxDelta)
	}
}

func TestSmoothElevation_ShortInput(t *testing.T) {
	// < 3 points -> return copy
	alt := []float64{500, 510}
	dist := []float64{0, 10}
	result := SmoothElevation(alt, dist)
	if len(result) != 2 || result[0] != 500 || result[1] != 510 {
		t.Errorf("short input should be returned unchanged")
	}
}

func TestSmoothElevation_NonUniformSpacing(t *testing.T) {
	// Non-uniform spacing (Smart Recording)
	// 1m, 1m, 50m, 1m, 1m -> Resampling must handle this
	alt := []float64{500, 501, 502, 510, 511}
	dist := []float64{0, 1, 2, 52, 53}
	result := SmoothElevation(alt, dist)
	if len(result) != len(alt) {
		t.Fatalf("length mismatch: got %d, want %d", len(result), len(alt))
	}
	// No NaN/Inf
	for i, v := range result {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("point %d: got NaN/Inf", i)
		}
	}
}

func TestSmoothElevation_PreservesLength(t *testing.T) {
	alt := make([]float64, 500)
	dist := make([]float64, 500)
	for i := range 500 {
		dist[i] = float64(i) * 3.0
		alt[i] = 500 + 50*math.Sin(float64(i)*0.1)
	}
	result := SmoothElevation(alt, dist)
	if len(result) != 500 {
		t.Fatalf("length: got %d, want 500", len(result))
	}
}

func TestDetectElevationSource_Barometric(t *testing.T) {
	// Barometer: small deltas (< 0.5m)
	alt := make([]float64, 100)
	dist := make([]float64, 100)
	for i := range 100 {
		dist[i] = float64(i) * 5.0
		alt[i] = 500 + float64(i)*0.1 // gentle climb, 0.1m per point
	}
	source := DetectElevationSource(alt, dist)
	if source != "barometric" {
		t.Errorf("got %q, want barometric", source)
	}
}

func TestDetectElevationSource_GPS(t *testing.T) {
	// GPS: large deltas (> 2m)
	alt := make([]float64, 100)
	dist := make([]float64, 100)
	for i := range 100 {
		dist[i] = float64(i) * 5.0
		alt[i] = 500 + float64(i)*0.5
		if i%2 == 0 {
			alt[i] += 5.0 // strong noise
		}
	}
	source := DetectElevationSource(alt, dist)
	if source != "gps_only" {
		t.Errorf("got %q, want gps_only", source)
	}
}

func TestHysteresisGainLoss_Basic(t *testing.T) {
	// 500 -> 510 -> 505 -> 515 with threshold 3m
	smoothed := []float64{500, 503, 506, 510, 508, 505, 508, 512, 515}
	gain, loss := HysteresisGainLoss(smoothed, 3.0)
	// Only changes > 3m count
	if gain < 10 || gain > 20 {
		t.Errorf("gain: got %.1f, want ~15", gain)
	}
	if loss < 2 || loss > 8 {
		t.Errorf("loss: got %.1f, want ~5", loss)
	}
}
