package enrichment

import (
	"testing"
)

// TestFilterElevationGainLossEmpty validates empty input
func TestFilterElevationGainLossEmpty(t *testing.T) {
	elevations := []float64{}
	distances := []float64{}
	gain, loss := FilterElevationGainLoss(elevations, distances, 5.0)
	if gain != 0 || loss != 0 {
		t.Errorf("Empty: expected 0, 0, got %.3f, %.3f", gain, loss)
	}
}

// TestFilterElevationGainLossSinglePoint validates single point
func TestFilterElevationGainLossSinglePoint(t *testing.T) {
	elevations := []float64{100.0}
	distances := []float64{0}
	gain, loss := FilterElevationGainLoss(elevations, distances, 5.0)
	if gain != 0 || loss != 0 {
		t.Errorf("Single point: expected 0, 0, got %.3f, %.3f", gain, loss)
	}
}

// TestFilterElevationGainLossFallbackDistances validates that nil distances
// triggers the fallback (5m per point estimation).
func TestFilterElevationGainLossFallbackDistances(t *testing.T) {
	// Monotonic climb: should produce gain > 0
	elevations := make([]float64, 50)
	for i := range 50 {
		elevations[i] = 100.0 + float64(i)
	}
	gain, loss := FilterElevationGainLoss(elevations, nil, 5.0)
	if gain < 10 {
		t.Errorf("Fallback distances: expected gain > 10, got %.3f", gain)
	}
	_ = loss
}

// TestFilterElevationGainLossClimb validates that a climb produces reasonable gain
func TestFilterElevationGainLossClimb(t *testing.T) {
	// Moderate climb with noise
	elevations := []float64{
		100.0, 101.5, 99.8, 102.2, 101.0,
		103.5, 102.3, 104.8, 103.2, 106.0,
		105.5, 107.2, 106.0, 108.5, 107.2,
		109.8, 108.5, 111.0, 110.0, 112.5,
		111.5, 113.8, 112.8, 115.0, 114.2,
	}
	distances := make([]float64, len(elevations))
	for i := range distances {
		distances[i] = float64(i) * 5.0
	}

	gain, _ := FilterElevationGainLoss(elevations, distances, 5.0)
	// Net climb ~14m (100 -> 114.2), with hysteresis some loss is filtered
	if gain < 5.0 {
		t.Errorf("Climb: expected gain > 5m, got %.3fm", gain)
	}
}

// BenchmarkFilterElevationGainLoss benchmarks full pipeline
func BenchmarkFilterElevationGainLoss(b *testing.B) {
	elevations := make([]float64, 3600)
	distances := make([]float64, 3600)
	for i := range 3600 {
		elevations[i] = 500.0 + float64(i%100)/10.0
		distances[i] = float64(i) * 5.0
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FilterElevationGainLoss(elevations, distances, 5.0)
	}
}
