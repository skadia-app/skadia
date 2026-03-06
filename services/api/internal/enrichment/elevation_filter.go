package enrichment

// SavitzkyGolayCoefficients stores precomputed coefficients for Savitzky-Golay filter.
// Using polynomial degree 2 (quadratic), which smooths noise while preserving peaks/valleys.
var (
	// Window size 5, degree 2: normalized coefficients
	coefficientsWindow5 = [5]float64{-0.086, 0.343, 0.486, 0.343, -0.086}

	// Window size 7, degree 2: normalized coefficients
	coefficientsWindow7 = [7]float64{-0.095, 0.103, 0.235, 0.314, 0.235, 0.103, -0.095}

	// Window size 11, degree 2: normalized coefficients
	// Calculated from Savitzky-Golay formula for degree 2, frame length 11
	coefficientsWindow11 = [11]float64{-0.094, -0.059, 0.044, 0.171, 0.291, 0.341, 0.291, 0.171, 0.044, -0.059, -0.094}
)

// DetermineFilterWindow selects window size based on sport type and average speed.
// Walking (< 2 m/s): larger window (11) for more aggressive smoothing
// Cycling/Running (>= 2 m/s): smaller window (5) to preserve actual terrain variations
func DetermineFilterWindow(avgSpeedMS float64) int {
	// < 2 m/s ≈ < 7.2 km/h → walking pace
	if avgSpeedMS < 2.0 {
		return 11
	}
	// < 5 m/s ≈ < 18 km/h → slower cycling/trail running
	if avgSpeedMS < 5.0 {
		return 7
	}
	// >= 5 m/s: default window (cycling, running, etc.)
	return 5
}

// SavitzkyGolayFilter applies a Savitzky-Golay lowpass filter to elevation data.
// This reduces GPS noise (±5–15m per point) while preserving real elevation changes.
//
// The algorithm:
// 1. Takes a sliding window of points
// 2. Applies precomputed polynomial coefficients
// 3. Outputs smoothed elevation at center of window
// 4. Edge points (outside window) are handled by reflection
//
// Parameters:
//   - elevations: array of elevation values in meters
//   - window: filter window size (5, 7, or 11); must be odd
//
// Returns: smoothed elevation array of same length
func SavitzkyGolayFilter(elevations []float64, window int) []float64 {
	n := len(elevations)

	// Edge case: not enough points for filtering
	if n < window {
		// Return copy of original if too short to filter
		result := make([]float64, n)
		copy(result, elevations)
		return result
	}

	// Ensure valid window size
	if window != 5 && window != 7 && window != 11 {
		window = 7 // default
	}

	result := make([]float64, n)
	coeffs := getCoefficients(window)

	halfWindow := window / 2

	for i := 0; i < n; i++ {
		// Determine the bounds for this point
		start := i - halfWindow

		// Handle edge reflections (reflect beyond boundaries)
		smoothedValue := 0.0
		for j := 0; j < window; j++ {
			idx := start + j

			// Reflect at boundaries if outside range
			if idx < 0 {
				idx = -idx - 1 // Reflect: -1 → 0, -2 → 1, etc.
			}
			if idx >= n {
				idx = 2*(n-1) - idx // Reflect: n → n-2, n+1 → n-3, etc.
			}

			// Clamp to valid range as fallback
			if idx < 0 {
				idx = 0
			}
			if idx >= n {
				idx = n - 1
			}

			smoothedValue += coeffs[j] * elevations[idx]
		}

		result[i] = smoothedValue
	}

	return result
}

// getCoefficients returns the appropriate coefficient array for the given window size.
func getCoefficients(window int) []float64 {
	switch window {
	case 5:
		return coefficientsWindow5[:]
	case 7:
		return coefficientsWindow7[:]
	case 11:
		return coefficientsWindow11[:]
	default:
		return coefficientsWindow7[:]
	}
}

// FilterElevationGainLoss applies Savitzky-Golay filter to elevation array
// and recalculates elevation gain and loss.
//
// This is the main entry point used by GPX/TCX parsers.
func FilterElevationGainLoss(elevations []float64, avgSpeedMS float64) (elevGain, elevLoss float64) {
	// Empty array → return zeros
	if len(elevations) == 0 {
		return 0, 0
	}

	// Single point → no gain/loss
	if len(elevations) == 1 {
		return 0, 0
	}

	// Determine appropriate window size
	window := DetermineFilterWindow(avgSpeedMS)

	// Apply Savitzky-Golay filter
	filtered := SavitzkyGolayFilter(elevations, window)

	// Calculate elevation gain/loss from filtered data
	for i := 1; i < len(filtered); i++ {
		diff := filtered[i] - filtered[i-1]
		if diff > 0 {
			elevGain += diff
		} else {
			elevLoss -= diff
		}
	}

	return elevGain, elevLoss
}
