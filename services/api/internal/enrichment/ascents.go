package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

const (
	ascentMinGradientPct  = 3.0   // default minimum average gradient (%)
	ascentMinElevationM   = 20.0  // default minimum elevation gain (m)
	ascentMinLengthM      = 100.0 // minimum segment length (m)
	ascentSmoothingWindow = 21    // Savitzky-Golay window size (reduced for Smart Recording)
	ascentPolyOrder       = 3     // polynomial order for SG filter
)

// enrichStreamAscents is the Go port of services/ml/enrichment/ascents.py.
//
// Algorithm:
//  1. Load per-user thresholds from user_enrichment_settings.
//  2. Load altitude + distance stream for the activity.
//  3. Apply Savitzky-Golay (window=51, polyorder=3) to smooth altitude.
//  4. Detect ascending segments and filter by elevation gain, length, gradient.
//  5. DELETE existing activity_climbs + INSERT fresh results.
func enrichStreamAscents(ctx context.Context, db *database.DB, userID, activityID string) error {
	// 1. Load user thresholds.
	minGradient := ascentMinGradientPct
	minElevation := ascentMinElevationM

	var gradPct, elevM *float64
	row := db.Pool.QueryRow(ctx, `
		SELECT climb_min_gradient_pct, climb_min_elevation_m
		FROM user_enrichment_settings
		WHERE user_id = $1
	`, userID)
	if err := row.Scan(&gradPct, &elevM); err == nil {
		if gradPct != nil {
			minGradient = *gradPct
		}
		if elevM != nil {
			minElevation = *elevM
		}
	}
	// Ignore "no rows" — defaults remain.

	// 2. Load altitude + distance stream.
	rows, err := db.Pool.Query(ctx, `
		SELECT altitude_m, distance_m
		FROM activity_streams
		WHERE activity_id = $1
		  AND altitude_m IS NOT NULL
		  AND distance_m IS NOT NULL
		ORDER BY time
	`, activityID)
	if err != nil {
		return fmt.Errorf("ascents: query stream: %w", err)
	}

	var altitudes, distances []float64
	for rows.Next() {
		var alt, dist float64
		if err := rows.Scan(&alt, &dist); err != nil {
			rows.Close()
			return fmt.Errorf("ascents: scan stream: %w", err)
		}
		altitudes = append(altitudes, alt)
		distances = append(distances, dist)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("ascents: stream rows: %w", err)
	}

	// 3. Need an absolute minimum of 7 points for poly-3 SG filter.
	if len(altitudes) < 7 {
		return nil
	}

	// Apply Savitzky-Golay filter (window up to 51, polyorder=3).
	window := min(ascentSmoothingWindow, len(altitudes))
	if window%2 == 0 {
		window--
	}
	if window < 5 {
		return nil
	}

	smoothed := sgFilter(altitudes, window, ascentPolyOrder)

	// 4. Find ascending segments and filter.
	type ascent struct {
		startDistM    float64
		endDistM      float64
		lengthM       float64
		elevationGain float64
		avgGradient   float64
		maxGradient   float64
	}

	var ascents []ascent
	inAscent := false
	startIdx := 0

	maybeAdd := func(si, ei int) {
		elevGain := smoothed[ei] - smoothed[si]
		lengthM := distances[ei] - distances[si]
		if elevGain < minElevation || lengthM < ascentMinLengthM {
			return
		}
		avgGrad := 0.0
		if lengthM > 0 {
			avgGrad = elevGain / lengthM * 100
		}
		if avgGrad < minGradient {
			return
		}

		maxGrad := 0.0
		for i := si; i < ei; i++ {
			segDist := distances[i+1] - distances[i]
			if segDist > 0 {
				g := (smoothed[i+1] - smoothed[i]) / segDist * 100
				if g > maxGrad {
					maxGrad = g
				}
			}
		}

		ascents = append(ascents, ascent{
			startDistM:    distances[si],
			endDistM:      distances[ei],
			lengthM:       lengthM,
			elevationGain: elevGain,
			avgGradient:   math.Round(avgGrad*10) / 10,
			maxGradient:   math.Round(maxGrad*10) / 10,
		})
	}

	for i := 1; i < len(smoothed); i++ {
		climbing := smoothed[i] > smoothed[i-1]
		if climbing && !inAscent {
			startIdx = i - 1
			inAscent = true
		} else if !climbing && inAscent {
			maybeAdd(startIdx, i)
			inAscent = false
		}
	}
	// Handle ascent that extends to end.
	if inAscent {
		maybeAdd(startIdx, len(smoothed)-1)
	}

	// 5. Delete existing climbs + insert fresh results.
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_climbs WHERE activity_id = $1",
		activityID,
	); err != nil {
		return fmt.Errorf("ascents: delete existing climbs: %w", err)
	}

	if len(ascents) == 0 {
		return nil
	}

	for idx, a := range ascents {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_climbs
			    (activity_id, climb_number, start_distance_m, end_distance_m,
			     length_m, elevation_gain_m, avg_gradient_pct, max_gradient_pct,
			     category)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, activityID, idx+1,
			a.startDistM, a.endDistM, a.lengthM, a.elevationGain,
			a.avgGradient, a.maxGradient,
			classifyClimb(a.elevationGain, a.avgGradient),
		); err != nil {
			slog.Warn("ascents: insert climb failed",
				"activity_id", activityID,
				"climb_number", idx+1,
				"error", err,
			)
		}
	}

	slog.Info("ascents: enrichment complete",
		"activity_id", activityID,
		"climbs", len(ascents),
	)
	return nil
}

// classifyClimb returns a standard cycling climb category (HC, 1–4) based on
// elevation gain (meters) and average gradient (percent).
func classifyClimb(elevationGain, avgGradient float64) string {
	switch {
	case elevationGain >= 1500 || (elevationGain >= 1000 && avgGradient >= 7):
		return "HC"
	case elevationGain >= 600 || (elevationGain >= 400 && avgGradient >= 7):
		return "1"
	case elevationGain >= 300 || (elevationGain >= 200 && avgGradient >= 7):
		return "2"
	case elevationGain >= 150 || (elevationGain >= 100 && avgGradient >= 7):
		return "3"
	default:
		return "4"
	}
}

// sgFilter applies a Savitzky-Golay filter with arbitrary odd window and polynomial order.
// It computes the center-point coefficient vector once and applies it with edge reflection.
// This is the general implementation required for window=51, polyorder=3 used in ascents.
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
// The coefficient vector c satisfies: c = (A^T A)^{-1} A^T e_m
// where A is the Vandermonde matrix of positions [-half..+half] and e_m
// is the unit vector selecting the center row of the fit (m = 0).
// Equivalently, c[j] = (A (A^T A)^{-1} A^T)_{center,j} which equals
// the j-th element of the hat matrix row for the center point.
//
// We compute it numerically using Gram-Schmidt orthogonalization
// (equivalent to the closed-form Gram polynomial approach).
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

	// Compute (A^T A)^{-1} via Cholesky or LU — use LU with partial pivoting.
	inv := matInverse(AtA, m)

	// The filter coefficients for center point (position 0) are:
	// c[j] = sum_k inv[0][k] * A[j][k]   (k=0..m-1, j=0..window-1)
	// This is row 0 of (A (A^T A)^{-1} A^T) which gives the hat matrix row.
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
