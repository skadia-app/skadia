package enrichment

// FilterElevationGainLoss applies the unified elevation filter pipeline:
// SmoothElevation (resample + SG) -> DetectElevationSource -> HysteresisGainLoss.
//
// The distances parameter is required for distance-based resampling.
// If distances is nil or empty, falls back to point-index-based estimation
// (assuming 5m per point — less accurate but prevents panic).
func FilterElevationGainLoss(elevations, distances []float64, avgSpeedMS float64) (elevGain, elevLoss float64) {
	if len(elevations) < 2 {
		return 0, 0
	}

	// Fallback: estimate distances if not provided (legacy callers)
	if len(distances) == 0 || len(distances) != len(elevations) {
		distances = make([]float64, len(elevations))
		for i := range distances {
			distances[i] = float64(i) * 5.0
		}
	}

	smoothed := SmoothElevation(elevations, distances)
	source := DetectElevationSource(elevations, distances)
	threshold := HysteresisThreshold(source)
	return HysteresisGainLoss(smoothed, threshold)
}
