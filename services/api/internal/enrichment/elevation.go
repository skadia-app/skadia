package enrichment

import (
	"math"
	"slices"
)

const (
	resampleIntervalM = 5.0 // uniform resampling distance in meters
	sgWindowResampled = 7   // SG window size in resampled points = 35m
	sgPolyResampled   = 2   // quadratic polynomial (not cubic — avoids overfitting)
)

// SmoothElevation resamples elevation to uniform 5m distance intervals,
// applies a Savitzky-Golay filter, and maps results back to original indices.
//
// The resampling step solves the fundamental problem that SG filters assume
// uniform point spacing. GPS data has variable spacing (1Hz Garmin vs. Smart
// Recording), making point-count-based windows unreliable.
//
// Algorithm:
//
//	Phase 1: Linear interpolation to 5m grid
//	Phase 2: SG filter (window=7 = 35m, poly=2)
//	Phase 3: Linear interpolation back to original distance positions
func SmoothElevation(altitudes, distances []float64) []float64 {
	n := len(altitudes)
	if n < 3 {
		return slices.Clone(altitudes)
	}
	totalDist := distances[n-1] - distances[0]
	if totalDist <= 0 {
		return slices.Clone(altitudes)
	}

	// Phase 1: Resample to uniform 5m intervals.
	numResampled := max(2, int(totalDist/resampleIntervalM)+1)
	resampled := make([]float64, numResampled)
	origIdx := 0
	dist0 := distances[0]

	for i := range numResampled {
		targetDist := dist0 + float64(i)*resampleIntervalM

		// Advance origIdx to bracket targetDist (monotonic distances -> linear scan O(n))
		for origIdx < n-2 && distances[origIdx+1] < targetDist {
			origIdx++
		}

		// Linear interpolation between origIdx and origIdx+1
		segLen := distances[origIdx+1] - distances[origIdx]
		if segLen <= 0 {
			resampled[i] = altitudes[origIdx]
		} else {
			t := (targetDist - distances[origIdx]) / segLen
			t = max(0, min(1, t)) // clamp
			resampled[i] = altitudes[origIdx]*(1-t) + altitudes[origIdx+1]*t
		}
	}

	// Phase 2: Apply SG filter on uniformly-spaced data.
	window := min(sgWindowResampled, numResampled)
	if window%2 == 0 {
		window--
	}
	if window < 3 {
		return slices.Clone(altitudes)
	}
	smoothedResampled := sgFilter(resampled, window, sgPolyResampled)

	// Phase 3: Map back to original point positions.
	// Position in resampled array = (dist - dist0) / resampleIntervalM
	// -> direct calculation, O(n), no binary search needed.
	result := make([]float64, n)
	lastResampled := numResampled - 1
	for i := range n {
		rPos := (distances[i] - dist0) / resampleIntervalM
		lo := int(rPos)
		if lo >= lastResampled {
			result[i] = smoothedResampled[lastResampled]
		} else if lo < 0 {
			result[i] = smoothedResampled[0]
		} else {
			frac := rPos - float64(lo)
			result[i] = smoothedResampled[lo]*(1-frac) + smoothedResampled[lo+1]*frac
		}
	}
	return result
}

// DetectElevationSource estimates whether altitude data is from a barometric
// altimeter, DEM, or GPS-only based on the noise signature.
//
// The heuristic computes the median of absolute point-to-point altitude deltas
// (excluding stationary points where distance < 1m):
//   - < 0.5m -> barometric (very smooth, drift but no spikes)
//   - < 2.0m -> likely barometric or DEM
//   - >= 2.0m -> GPS-only (noisy, +/-5-15m random jumps)
//
// Flat terrain fallback: if >80% of deltas are < 0.1m, the source is
// indistinguishable -> returns "flat" (use conservative 3m threshold).
func DetectElevationSource(altitudes, distances []float64) string {
	n := len(altitudes)
	if n < 10 {
		return "unknown"
	}

	var deltas []float64
	var tinyCount int
	for i := 1; i < n; i++ {
		segDist := distances[i] - distances[i-1]
		if segDist < 1.0 {
			continue // skip stationary points
		}
		d := math.Abs(altitudes[i] - altitudes[i-1])
		deltas = append(deltas, d)
		if d < 0.1 {
			tinyCount++
		}
	}
	if len(deltas) < 5 {
		return "unknown"
	}

	// Flat terrain: >80% of deltas are < 0.1m
	if float64(tinyCount)/float64(len(deltas)) > 0.8 {
		return "flat"
	}

	slices.Sort(deltas)
	median := deltas[len(deltas)/2]

	switch {
	case median < 0.5:
		return "barometric"
	case median < 2.0:
		return "likely_barometric"
	default:
		return "gps_only"
	}
}

// HysteresisThreshold returns the elevation change threshold below which
// altitude fluctuations are ignored for gain/loss summation.
func HysteresisThreshold(source string) float64 {
	switch source {
	case "barometric", "likely_barometric":
		return 2.0
	case "dem":
		return 3.0
	case "gps_only":
		return 5.0
	case "flat":
		return 3.0 // conservative default for indistinguishable sources
	default:
		return 3.0
	}
}

// HysteresisGainLoss computes elevation gain and loss using a deadband
// threshold. Altitude fluctuations below the threshold are ignored, which
// prevents noise from being double-counted as both gain and loss.
//
// The caller chooses the threshold based on altitude source and desired
// noise tolerance.
func HysteresisGainLoss(smoothed []float64, threshold float64) (gain, loss float64) {
	if len(smoothed) < 2 {
		return 0, 0
	}
	refAlt := smoothed[0]
	for i := 1; i < len(smoothed); i++ {
		delta := smoothed[i] - refAlt
		if delta > threshold {
			gain += delta
			refAlt = smoothed[i]
		} else if delta < -threshold {
			loss -= delta // loss is positive
			refAlt = smoothed[i]
		}
	}
	return
}

// sgFilter applies a Savitzky-Golay filter with arbitrary odd window and polynomial order.
// It computes the center-point coefficient vector once and applies it with edge reflection.
func sgFilter(data []float64, window, polyOrder int) []float64 {
	n := len(data)
	coeffs := sgCoefficients(window, polyOrder)
	half := window / 2
	result := make([]float64, n)

	for i := range n {
		val := 0.0
		for j, c := range coeffs {
			idx := i - half + j
			// Reflect at boundaries.
			if idx < 0 {
				idx = -idx - 1
			}
			if idx >= n {
				idx = 2*(n-1) - idx
			}
			// Clamp as fallback.
			if idx < 0 {
				idx = 0
			}
			if idx >= n {
				idx = n - 1
			}
			val += c * data[idx]
		}
		result[i] = val
	}
	return result
}

// sgCoefficients computes the Savitzky-Golay convolution coefficients for the
// center point of a window of size `window` and polynomial order `polyOrder`.
//
// Method: least-squares polynomial fit via normal equations.
func sgCoefficients(window, polyOrder int) []float64 {
	half := window / 2
	m := polyOrder + 1 // number of basis polynomials

	// Build Vandermonde matrix A (window × m): A[i][k] = (i-half)^k
	A := make([][]float64, window)
	for i := range window {
		x := float64(i - half)
		A[i] = make([]float64, m)
		xk := 1.0
		for k := range m {
			A[i][k] = xk
			xk *= x
		}
	}

	// Compute A^T A (m × m)
	AtA := make([][]float64, m)
	for i := range m {
		AtA[i] = make([]float64, m)
		for j := range m {
			for r := range window {
				AtA[i][j] += A[r][i] * A[r][j]
			}
		}
	}

	// Compute (A^T A)^{-1} via Gaussian elimination with partial pivoting.
	inv := matInverse(AtA, m)

	// The filter coefficients for center point (position 0) are:
	// c[j] = sum_k inv[0][k] * A[j][k]   (k=0..m-1, j=0..window-1)
	coeffs := make([]float64, window)
	for j := range window {
		for k := range m {
			coeffs[j] += inv[0][k] * A[j][k]
		}
	}
	return coeffs
}

// matInverse computes the inverse of an m×m matrix using Gaussian elimination
// with partial pivoting. Returns an m×m inverse matrix.
func matInverse(a [][]float64, m int) [][]float64 {
	// Augment with identity.
	aug := make([][]float64, m)
	for i := range m {
		aug[i] = make([]float64, 2*m)
		copy(aug[i], a[i])
		aug[i][m+i] = 1.0
	}

	// Forward elimination.
	for col := range m {
		// Find pivot.
		pivot := col
		maxVal := math.Abs(aug[col][col])
		for row := col + 1; row < m; row++ {
			if v := math.Abs(aug[row][col]); v > maxVal {
				maxVal = v
				pivot = row
			}
		}
		aug[col], aug[pivot] = aug[pivot], aug[col]

		// Scale pivot row.
		scale := aug[col][col]
		if math.Abs(scale) < 1e-14 {
			continue
		}
		for j := range 2 * m {
			aug[col][j] /= scale
		}

		// Eliminate column.
		for row := range m {
			if row == col {
				continue
			}
			factor := aug[row][col]
			for j := range 2 * m {
				aug[row][j] -= factor * aug[col][j]
			}
		}
	}

	// Extract inverse.
	inv := make([][]float64, m)
	for i := range m {
		inv[i] = make([]float64, m)
		copy(inv[i], aug[i][m:])
	}
	return inv
}
