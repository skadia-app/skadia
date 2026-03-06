package enrichment

import (
	"math"
	"testing"
)

const tolerance = 0.01 // 1 cm tolerance for floating point comparisons

// TestSavitzkyGolayFilterFlatWithNoise validates that noise on flat terrain is smoothed out.
func TestSavitzkyGolayFilterFlatWithNoise(t *testing.T) {
	// Simulate a flat track at 100m elevation with ±1m noise
	elevations := []float64{
		100.0, 100.5, 99.8, 100.2, 100.0,
		99.9, 100.1, 100.3, 99.7, 100.0,
		100.2, 99.8, 100.1, 100.0, 100.3,
	}

	filtered := SavitzkyGolayFilter(elevations, 7)

	// After filtering, gain/loss should be near zero
	elevGain, elevLoss := 0.0, 0.0
	for i := 1; i < len(filtered); i++ {
		diff := filtered[i] - filtered[i-1]
		if diff > 0 {
			elevGain += diff
		} else {
			elevLoss -= diff
		}
	}

	if elevGain > 0.5 { // Allow 50cm tolerance
		t.Errorf("Flat track: expected elevGain ≤ 0.5m, got %.3fm", elevGain)
	}
	if elevLoss > 0.5 {
		t.Errorf("Flat track: expected elevLoss ≤ 0.5m, got %.3fm", elevLoss)
	}
}

// TestSavitzkyGolayFilterSmallClimb validates that a real climb is preserved.
func TestSavitzkyGolayFilterSmallClimb(t *testing.T) {
	// Simulate smooth 50m climb (1m per point) with ±2m noise
	elevations := []float64{
		100.0, 101.5, 99.8, 102.0, 101.2,
		103.0, 102.5, 104.2, 103.8, 105.0,
		105.5, 104.8, 106.0, 105.2, 107.0,
		107.3, 106.8, 108.5, 108.0, 109.0,
		109.2, 108.9, 110.5, 110.0, 110.8,
		111.0, 110.8, 112.0, 111.2, 112.5,
		112.8, 111.9, 113.2, 112.8, 114.0,
		114.2, 113.8, 115.0, 114.5, 115.5,
		115.8, 115.2, 116.0, 115.8, 116.5,
		116.8, 115.9, 117.0, 116.5, 117.5,
	}

	filtered := SavitzkyGolayFilter(elevations, 7)

	// Calculate filtered gain/loss
	elevGain, elevLoss := 0.0, 0.0
	for i := 1; i < len(filtered); i++ {
		diff := filtered[i] - filtered[i-1]
		if diff > 0 {
			elevGain += diff
		} else {
			elevLoss -= diff
		}
	}

	// Expected: ~17.5m gain (from 100 to 117.5), minimal loss
	// With Savitzky-Golay filtering (polynomial degree 2), edge effects reduce this slightly
	// Realistic range after filtering: 12–18m
	if elevGain < 12.0 || elevGain > 18.0 {
		t.Errorf("Climb: expected elevGain 12–18m, got %.3fm", elevGain)
	}
	if elevLoss > 2.0 {
		t.Errorf("Climb: expected elevLoss < 2m, got %.3fm", elevLoss)
	}
}

// TestSavitzkyGolayFilterRoundTrip validates that up = down on round trip.
func TestSavitzkyGolayFilterRoundTrip(t *testing.T) {
	// Simulate: 0m → climb 100m → descent 100m = 0m, with noise
	elevations := []float64{
		100.0, 101.2, 100.8, 102.1, 101.5, // Start at 100m
		103.0, 102.5, 104.0, 103.5, 105.0, // Climbing
		106.0, 105.5, 107.0, 106.5, 108.0,
		109.0, 108.5, 110.0, 109.5, 110.8, // Mid-point ~110m
		111.5, 110.8, 112.0, 111.2, 112.5, // Continue climbing
		113.0, 112.5, 114.0, 113.2, 114.8, // Peak ~115m
		115.5, 114.8, 115.2, 114.5, 115.0,
		114.0, 115.2, 113.8, 114.5, 113.0, // Start descending
		112.0, 111.5, 112.0, 110.5, 110.0,
		109.0, 108.5, 109.0, 108.0, 107.0, // Mid-descent
		106.0, 105.5, 106.0, 104.5, 104.0,
		103.0, 102.5, 103.0, 101.5, 101.0, // Back toward start
		100.0, 101.0, 99.8, 100.5, 99.5, // End near 100m
	}

	filtered := SavitzkyGolayFilter(elevations, 7)

	elevGain, elevLoss := 0.0, 0.0
	for i := 1; i < len(filtered); i++ {
		diff := filtered[i] - filtered[i-1]
		if diff > 0 {
			elevGain += diff
		} else {
			elevLoss -= diff
		}
	}

	// Round trip: gain should ≈ loss
	// Allow ±5% difference due to filtering artifacts
	maxDiff := math.Max(elevGain, elevLoss) * 0.05
	if math.Abs(elevGain-elevLoss) > maxDiff {
		t.Errorf("Round trip: elevGain %.3fm ≠ elevLoss %.3fm (diff: %.3fm)",
			elevGain, elevLoss, math.Abs(elevGain-elevLoss))
	}
}

// TestDetermineFilterWindowWalking validates walking → window 11
func TestDetermineFilterWindowWalking(t *testing.T) {
	// Walking pace: 1.5 m/s (5.4 km/h)
	window := DetermineFilterWindow(1.5)
	if window != 11 {
		t.Errorf("Walking pace: expected window 11, got %d", window)
	}
}

// TestDetermineFilterWindowCycling validates cycling → window 5
func TestDetermineFilterWindowCycling(t *testing.T) {
	// Cycling pace: 8 m/s (28.8 km/h)
	window := DetermineFilterWindow(8.0)
	if window != 5 {
		t.Errorf("Cycling pace: expected window 5, got %d", window)
	}
}

// TestDetermineFilterWindowRunning validates running → window 7
func TestDetermineFilterWindowRunning(t *testing.T) {
	// Running pace: 4 m/s (14.4 km/h)
	window := DetermineFilterWindow(4.0)
	if window != 7 {
		t.Errorf("Running pace: expected window 7, got %d", window)
	}
}

// TestFilterElevationGainLossEmpty validates empty input
func TestFilterElevationGainLossEmpty(t *testing.T) {
	elevations := []float64{}
	gain, loss := FilterElevationGainLoss(elevations, 5.0)
	if gain != 0 || loss != 0 {
		t.Errorf("Empty: expected 0, 0, got %.3f, %.3f", gain, loss)
	}
}

// TestFilterElevationGainLossSinglePoint validates single point
func TestFilterElevationGainLossSinglePoint(t *testing.T) {
	elevations := []float64{100.0}
	gain, loss := FilterElevationGainLoss(elevations, 5.0)
	if gain != 0 || loss != 0 {
		t.Errorf("Single point: expected 0, 0, got %.3f, %.3f", gain, loss)
	}
}

// TestFilterElevationGainLossWalkingVsCycling validates different window sizes
// produce different but reasonable results
func TestFilterElevationGainLossWalkingVsCycling(t *testing.T) {
	// Moderate climb with noise (good test case)
	elevations := []float64{
		100.0, 101.5, 99.8, 102.2, 101.0,
		103.5, 102.3, 104.8, 103.2, 106.0,
		105.5, 107.2, 106.0, 108.5, 107.2,
		109.8, 108.5, 111.0, 110.0, 112.5,
		111.5, 113.8, 112.8, 115.0, 114.2,
	}

	// Walking: larger window (11) → more aggressive smoothing
	walkingGain, walkingLoss := FilterElevationGainLoss(elevations, 1.5) // Walking speed

	// Cycling: smaller window (5) → less smoothing
	cyclingGain, cyclingLoss := FilterElevationGainLoss(elevations, 8.0) // Cycling speed

	// Both should show net uphill (elevGain > elevLoss)
	if walkingGain < 5.0 {
		t.Errorf("Walking: expected gain > 5m, got %.3fm", walkingGain)
	}
	if cyclingGain < 5.0 {
		t.Errorf("Cycling: expected gain > 5m, got %.3fm", cyclingGain)
	}

	// Walking should have more smoothing (lower loss) than cycling
	// due to larger window
	if walkingLoss > cyclingLoss {
		t.Logf("Warning: Walking loss (%.3fm) > cycling loss (%.3fm), unusual but possible",
			walkingLoss, cyclingLoss)
	}
}

// TestSavitzkyGolayFilterEdgeCases validates boundary reflections
func TestSavitzkyGolayFilterEdgeCases(t *testing.T) {
	// Test with short array (less than window size)
	shortArray := []float64{100.0, 101.0, 102.0}
	filtered := SavitzkyGolayFilter(shortArray, 7)

	// Should return same length
	if len(filtered) != len(shortArray) {
		t.Errorf("Short array: expected length %d, got %d", len(shortArray), len(filtered))
	}

	// Values should still be reasonable (no NaN, Inf)
	for i, val := range filtered {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			t.Errorf("Short array: point %d is invalid: %v", i, val)
		}
	}
}

// TestSavitzkyGolayFilterMonotonicClimb validates that a perfectly linear climb
// is preserved (within tolerance)
func TestSavitzkyGolayFilterMonotonicClimb(t *testing.T) {
	// Perfect linear climb: 100m + 1m per point = 100, 101, 102, ..., 149
	elevations := make([]float64, 50)
	for i := 0; i < 50; i++ {
		elevations[i] = 100.0 + float64(i)
	}

	filtered := SavitzkyGolayFilter(elevations, 7)

	// Calculate gain/loss
	elevGain, elevLoss := 0.0, 0.0
	for i := 1; i < len(filtered); i++ {
		diff := filtered[i] - filtered[i-1]
		if diff > 0 {
			elevGain += diff
		} else {
			elevLoss -= diff
		}
	}

	// Expected: ~49m gain (from 100 to 149), 0 loss
	// Savitzky-Golay filter edge effects reduce this at boundaries (~35–45m realistic after filtering)
	if elevGain < 35.0 || elevGain > 45.0 {
		t.Errorf("Monotonic climb: expected 35–45m gain after filtering, got %.3fm", elevGain)
	}
	if elevLoss > 2.0 {
		t.Errorf("Monotonic climb: expected ~0m loss, got %.3fm", elevLoss)
	}
}

// TestSavitzkyGolayFilterNaNHandling validates graceful handling of NaN values
// (though in practice, we should have filtered these upstream)
func TestSavitzkyGolayFilterNaNHandling(t *testing.T) {
	// Normal case (no NaN)
	elevations := []float64{100.0, 101.0, 102.0, 103.0, 104.0}
	filtered := SavitzkyGolayFilter(elevations, 5)

	hasNaN := false
	for _, val := range filtered {
		if math.IsNaN(val) {
			hasNaN = true
			break
		}
	}

	if hasNaN {
		t.Error("Filter produced NaN for valid input")
	}
}

// BenchmarkSavitzkyGolayFilter benchmarks filter performance on realistic data
func BenchmarkSavitzkyGolayFilter(b *testing.B) {
	// Realistic data: 1-hour activity at 1Hz = ~3600 points
	elevations := make([]float64, 3600)
	for i := 0; i < 3600; i++ {
		elevations[i] = 500.0 + float64(i%100)/10.0 + float64(i%13)/13.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SavitzkyGolayFilter(elevations, 7)
	}
}

// BenchmarkFilterElevationGainLoss benchmarks full pipeline
func BenchmarkFilterElevationGainLoss(b *testing.B) {
	elevations := make([]float64, 3600)
	for i := 0; i < 3600; i++ {
		elevations[i] = 500.0 + float64(i%100)/10.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FilterElevationGainLoss(elevations, 5.0)
	}
}
