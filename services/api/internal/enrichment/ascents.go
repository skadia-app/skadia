package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

const (
	ascentMinGradientPct = 3.0   // default minimum average gradient (%)
	ascentMinElevationM  = 20.0  // default minimum elevation gain (m)
	ascentMinLengthM     = 100.0 // minimum segment length (m)
)

// ascent holds the intermediate data for a detected ascending segment.
type ascent struct {
	startDistM    float64
	endDistM      float64
	lengthM       float64
	elevationGain float64
	avgGradient   float64
	maxGradient   float64
}

// enrichStreamAscents is the Go port of services/ml/enrichment/ascents.py.
//
// Algorithm:
//  1. Load per-user thresholds from user_enrichment_settings.
//  2. Load sport group for sport-adaptive classification.
//  3. Filter altitude + distance points from the shared stream (both must be non-nil).
//  4. Apply unified SmoothElevation (resample 5m + SG) to smooth altitude.
//  5. Detect ascending segments and filter by elevation gain, length, gradient.
//  6. Merge adjacent ascents separated by shallow dips (<3m, <50m).
//  7. DELETE existing activity_climbs + INSERT fresh results.
func enrichStreamAscents(ctx context.Context, db *database.DB, userID, activityID string, stream []EnrichmentStreamPoint) error {
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

	// 2. Load sport group for sport-adaptive classification.
	// Also load elevation_source: when 'dem', the device track was replaced with
	// DEM-derived elevation (3m hysteresis via FilterElevationGainLoss). Ascent
	// detection here uses SmoothElevation rather than HysteresisGainLoss, so
	// elevation_source does not change the algorithm directly — but it is loaded
	// here so future callers (e.g. a hysteresis-aware pass) can branch on it
	// without an additional DB round-trip.
	var sportType, elevationSource string
	_ = db.Pool.QueryRow(ctx,
		"SELECT sport_type, COALESCE(elevation_source, 'device') FROM activities WHERE id = $1",
		activityID,
	).Scan(&sportType, &elevationSource)
	_ = elevationSource // reserved for future hysteresis-aware path
	sportGroup := "generic"
	if def, ok := sport.GetSportDef(sportType); ok {
		sportGroup = def.SegmentGroup
	}

	// 3. Filter to points where altitude_m and distance_m are both non-nil.
	var altitudes, distances []float64
	for _, sp := range stream {
		if sp.AltitudeM == nil || sp.DistanceM == nil {
			continue
		}
		altitudes = append(altitudes, *sp.AltitudeM)
		distances = append(distances, *sp.DistanceM)
	}

	// 4. Need an absolute minimum of 3 points for SmoothElevation.
	if len(altitudes) < 3 {
		return nil
	}

	// Apply unified elevation filter (resample 5m + SG window=7 poly=2).
	smoothed := SmoothElevation(altitudes, distances)

	// 5. Find ascending segments and filter.
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
			if segDist >= 5.0 { // min 5m (= resample interval)
				g := (smoothed[i+1] - smoothed[i]) / segDist * 100
				g = min(g, 200.0) // sanity cap: alpine Steilstufen bis ~150% real
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

	// 6. Merge adjacent ascents separated by shallow dips (<3m elevation, <50m distance).
	ascents = mergeShallowDips(ascents, smoothed, distances, 3.0, 50.0)

	// 7. Delete existing climbs + insert fresh results.
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
		sportClass := classifyClimbSportClass(a.elevationGain, a.avgGradient, sportGroup)
		var sportClassPtr *string
		if sportClass != "" {
			sportClassPtr = &sportClass
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_climbs
			    (activity_id, climb_number, start_distance_m, end_distance_m,
			     length_m, elevation_gain_m, avg_gradient_pct, max_gradient_pct,
			     category, sport_class)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, activityID, idx+1,
			a.startDistM, a.endDistM, a.lengthM, a.elevationGain,
			a.avgGradient, a.maxGradient,
			classifyClimb(a.elevationGain, a.avgGradient),
			sportClassPtr,
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
// This value is stored in activity_climbs.category for all sports
// (backward compatible with rules engine and gear.go which cast to int).
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

// classifyClimbSportClass returns a sport-specific classification.
// Stored in activity_climbs.sport_class (new column, does not break existing code).
// Returns "" for bike (cycling uses category HC/1-4) and unknown sport groups.
func classifyClimbSportClass(elevationGain, avgGradient float64, sportGroup string) string {
	switch sportGroup {
	case "foot":
		return classifyClimbHiking(avgGradient)
	case "bike":
		return "" // cycling uses category (HC/1-4), no separate sport_class
	default:
		return classifyClimbGeneric(elevationGain)
	}
}

// classifyClimbHiking returns a hiking-specific gradient classification (S1–S5).
func classifyClimbHiking(avgGradient float64) string {
	switch {
	case avgGradient >= 40:
		return "S5" // extrem steil
	case avgGradient >= 20:
		return "S4" // sehr steil
	case avgGradient >= 10:
		return "S3" // steil
	case avgGradient >= 5:
		return "S2" // moderat
	default:
		return "S1" // flach
	}
}

// classifyClimbGeneric returns a generic elevation-based classification.
func classifyClimbGeneric(elevationGain float64) string {
	switch {
	case elevationGain >= 500:
		return "schwer"
	case elevationGain >= 200:
		return "mittel"
	default:
		return "leicht"
	}
}

// mergeShallowDips merges adjacent ascents separated by dips < maxDipElevM
// elevation AND < maxDipDistM distance. Requires the smoothed elevation
// array and corresponding distance array to measure actual dip depth.
func mergeShallowDips(ascents []ascent, smoothed, distances []float64, maxDipElevM, maxDipDistM float64) []ascent {
	if len(ascents) < 2 {
		return ascents
	}

	// Build distance→index lookup for smoothed array (binary search).
	findIdx := func(distM float64) int {
		lo, hi := 0, len(distances)-1
		for lo < hi {
			mid := (lo + hi) / 2
			if distances[mid] < distM {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		return lo
	}

	merged := []ascent{ascents[0]}
	for i := 1; i < len(ascents); i++ {
		prev := &merged[len(merged)-1]
		curr := ascents[i]

		gapDist := curr.startDistM - prev.endDistM
		if gapDist < 0 || gapDist > maxDipDistM {
			merged = append(merged, curr)
			continue
		}

		// Measure actual dip depth from smoothed elevations.
		prevEndIdx := findIdx(prev.endDistM)
		currStartIdx := findIdx(curr.startDistM)
		if prevEndIdx >= len(smoothed) || currStartIdx >= len(smoothed) {
			merged = append(merged, curr)
			continue
		}

		endElev := smoothed[prevEndIdx]
		minElev := endElev
		for j := prevEndIdx; j <= currStartIdx && j < len(smoothed); j++ {
			if smoothed[j] < minElev {
				minElev = smoothed[j]
			}
		}
		dipElev := endElev - minElev

		if dipElev <= maxDipElevM {
			// Merge: extend prev to include curr.
			prev.endDistM = curr.endDistM
			prev.lengthM = prev.endDistM - prev.startDistM
			// Recalculate gain from smoothed data for the merged segment so that
			// additive accumulation errors (e.g. dip elevation counted twice) do
			// not inflate the result.
			startElev := smoothed[findIdx(prev.startDistM)]
			endElev := smoothed[findIdx(prev.endDistM)]
			prev.elevationGain = max(0, endElev-startElev)
			if prev.lengthM > 0 {
				prev.avgGradient = math.Round(prev.elevationGain/prev.lengthM*1000) / 10
			}
			prev.maxGradient = max(prev.maxGradient, curr.maxGradient)
		} else {
			merged = append(merged, curr)
		}
	}
	return merged
}
