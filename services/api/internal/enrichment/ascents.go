package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

const (
	ascentMinGradientPct = 5.0   // default minimum average gradient (%)
	ascentMinElevationM  = 20.0  // default minimum elevation gain (m)
	ascentMinLengthM     = 100.0 // minimum segment length (m)
	ascentMaxStreamGap   = 5 * time.Minute

	ascentMergeMinDipM                = 3.0
	ascentMergeRelativeDipFactor      = 0.15
	ascentMergeRelativeDipCapM        = 15.0
	ascentMergeMaxGapM                = 300.0
	ascentExtensionMinElevationFactor = 0.5
	ascentExtensionMinGradientFactor  = 0.7
	ascentMaxGradientWindowM          = 50.0

	ascentOutlierMaxSpanM           = 500.0
	ascentOutlierMinExcursionM      = 25.0
	ascentOutlierMinSteepLegPct     = 25.0
	ascentOutlierMaxEndpointDeltaM  = 25.0
	ascentOutlierReplacementPadding = 1
)

// ascent holds the intermediate data for a detected ascending segment.
type ascent struct {
	startIdx      int
	endIdx        int
	coreEndIdx    int
	coreEndDistM  float64
	startDistM    float64
	endDistM      float64
	lengthM       float64
	elevationGain float64
	avgGradient   float64
	maxGradient   float64
	strongCore    bool
}

// enrichStreamAscents is the Go port of services/ml/enrichment/ascents.py.
//
// Algorithm:
//  1. Load per-user thresholds from user_enrichment_settings.
//  2. Load sport group for sport-adaptive classification.
//  3. Filter altitude + distance points from the shared stream (both must be non-nil).
//  4. Apply unified SmoothElevation (resample 5m + SG) to smooth altitude.
//  5. Detect ascending segments and filter by elevation gain, length, gradient.
//  6. Merge adjacent ascent cores and extensions across shallow rolling dips.
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
		if gradPct != nil && *gradPct > 0 {
			minGradient = *gradPct
		}
		if elevM != nil && *elevM > 0 {
			minElevation = *elevM
		}
	}
	// Ignore "no rows" — defaults remain. Zero values are treated as "use default".

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

	ascents := detectAscentsFromStream(stream, minGradient, minElevation)

	// 7. Delete existing climbs + insert fresh results.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ascents: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		"DELETE FROM activity_climbs WHERE activity_id = $1",
		activityID,
	); err != nil {
		return fmt.Errorf("ascents: delete existing climbs: %w", err)
	}

	if len(ascents) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("ascents: commit empty result: %w", err)
		}
		return nil
	}

	for idx, a := range ascents {
		sportClass := classifyClimbSportClass(a.elevationGain, a.avgGradient, sportGroup)
		var sportClassPtr *string
		if sportClass != "" {
			sportClassPtr = &sportClass
		}

		if _, err := tx.Exec(ctx, `
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
			return fmt.Errorf("ascents: insert climb %d: %w", idx+1, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("ascents: commit climbs: %w", err)
	}

	slog.Info("ascents: enrichment complete",
		"activity_id", activityID,
		"climbs", len(ascents),
	)
	return nil
}

func detectAscentsFromStream(stream []EnrichmentStreamPoint, minGradient, minElevation float64) []ascent {
	var ascents []ascent
	var altitudes, distances []float64
	var lastTime time.Time

	flush := func() {
		if len(altitudes) >= 3 {
			ascents = append(ascents, detectAscentsFromSeries(altitudes, distances, minGradient, minElevation)...)
		}
		altitudes = nil
		distances = nil
		lastTime = time.Time{}
	}

	for _, sp := range stream {
		if sp.AltitudeM == nil || sp.DistanceM == nil {
			continue
		}
		if len(altitudes) > 0 && !lastTime.IsZero() && !sp.Time.IsZero() && sp.Time.Sub(lastTime) > ascentMaxStreamGap {
			flush()
		}
		altitudes = append(altitudes, *sp.AltitudeM)
		distances = append(distances, *sp.DistanceM)
		lastTime = sp.Time
	}
	flush()
	return ascents
}

func detectAscentsFromSeries(altitudes, distances []float64, minGradient, minElevation float64) []ascent {
	if len(altitudes) < 3 || len(altitudes) != len(distances) {
		return nil
	}

	altitudes = removeTransientAltitudeExcursions(altitudes, distances)
	smoothed := SmoothElevation(altitudes, distances)
	return detectAscentsFromSmoothed(smoothed, distances, minGradient, minElevation)
}

func removeTransientAltitudeExcursions(altitudes, distances []float64) []float64 {
	if len(altitudes) < 5 || len(altitudes) != len(distances) {
		return altitudes
	}

	filtered := append([]float64(nil), altitudes...)
	for i := 1; i < len(filtered)-1; i++ {
		leftIdx, rightIdx, ok := findTransientAltitudeExcursion(filtered, distances, i)
		if !ok {
			continue
		}
		interpolateAltitudeRange(filtered, distances, leftIdx, rightIdx)
		i = max(i, rightIdx-ascentOutlierReplacementPadding)
	}
	return filtered
}

func findTransientAltitudeExcursion(altitudes, distances []float64, extremumIdx int) (int, int, bool) {
	if extremumIdx <= 0 || extremumIdx >= len(altitudes)-1 {
		return 0, 0, false
	}

	prevDelta := altitudes[extremumIdx] - altitudes[extremumIdx-1]
	nextDelta := altitudes[extremumIdx+1] - altitudes[extremumIdx]
	if !(prevDelta < 0 && nextDelta > 0) && !(prevDelta > 0 && nextDelta < 0) {
		return 0, 0, false
	}
	isValley := prevDelta < 0 && nextDelta > 0

	leftIdx := excursionLeftAnchor(altitudes, distances, extremumIdx, isValley)
	rightIdx := excursionRightAnchor(altitudes, distances, extremumIdx, isValley)
	if leftIdx < 0 || rightIdx <= extremumIdx {
		return 0, 0, false
	}
	spanM := distances[rightIdx] - distances[leftIdx]
	if spanM <= 0 || spanM > ascentOutlierMaxSpanM {
		return 0, 0, false
	}

	leftAlt := altitudes[leftIdx]
	midAlt := altitudes[extremumIdx]
	rightAlt := altitudes[rightIdx]
	var leftExcursion, rightExcursion float64
	if isValley {
		leftExcursion = leftAlt - midAlt
		rightExcursion = rightAlt - midAlt
	} else {
		leftExcursion = midAlt - leftAlt
		rightExcursion = midAlt - rightAlt
	}
	if min(leftExcursion, rightExcursion) < ascentOutlierMinExcursionM {
		return 0, 0, false
	}
	if math.Abs(rightAlt-leftAlt) > ascentOutlierMaxEndpointDeltaM {
		return 0, 0, false
	}

	leftGradient := leftExcursion / (distances[extremumIdx] - distances[leftIdx]) * 100
	rightGradient := rightExcursion / (distances[rightIdx] - distances[extremumIdx]) * 100
	if min(leftGradient, rightGradient) < ascentOutlierMinSteepLegPct {
		return 0, 0, false
	}
	return leftIdx, rightIdx, true
}

func excursionLeftAnchor(altitudes, distances []float64, extremumIdx int, valley bool) int {
	bestIdx := extremumIdx - 1
	bestAlt := altitudes[bestIdx]
	minDist := distances[extremumIdx] - ascentOutlierMaxSpanM/2
	for i := extremumIdx - 2; i >= 0 && distances[i] >= minDist; i-- {
		if valley {
			if altitudes[i] >= bestAlt {
				bestAlt = altitudes[i]
				bestIdx = i
			}
			continue
		}
		if altitudes[i] <= bestAlt {
			bestAlt = altitudes[i]
			bestIdx = i
		}
	}
	return bestIdx
}

func excursionRightAnchor(altitudes, distances []float64, extremumIdx int, valley bool) int {
	bestIdx := extremumIdx + 1
	bestAlt := altitudes[bestIdx]
	maxDist := distances[extremumIdx] + ascentOutlierMaxSpanM/2
	for i := extremumIdx + 2; i < len(altitudes) && distances[i] <= maxDist; i++ {
		if valley {
			if altitudes[i] >= bestAlt {
				bestAlt = altitudes[i]
				bestIdx = i
			}
			continue
		}
		if altitudes[i] <= bestAlt {
			bestAlt = altitudes[i]
			bestIdx = i
		}
	}
	return bestIdx
}

func interpolateAltitudeRange(altitudes, distances []float64, leftIdx, rightIdx int) {
	if rightIdx <= leftIdx || leftIdx < 0 || rightIdx >= len(altitudes) {
		return
	}
	spanM := distances[rightIdx] - distances[leftIdx]
	if spanM <= 0 {
		return
	}
	leftAlt := altitudes[leftIdx]
	rightAlt := altitudes[rightIdx]
	for i := leftIdx + ascentOutlierReplacementPadding; i < rightIdx-ascentOutlierReplacementPadding+1; i++ {
		t := (distances[i] - distances[leftIdx]) / spanM
		t = max(0, min(1, t))
		altitudes[i] = leftAlt*(1-t) + rightAlt*t
	}
}

func detectAscentsFromSmoothed(smoothed, distances []float64, minGradient, minElevation float64) []ascent {
	if len(smoothed) < 3 || len(smoothed) != len(distances) {
		return nil
	}

	candidates := detectAscentCandidates(smoothed, distances)
	return mergeAndFilterAscentCandidates(candidates, smoothed, distances, minGradient, minElevation)
}

func detectAscentCandidates(smoothed, distances []float64) []ascent {
	var candidates []ascent
	inAscent := false
	startIdx := 0

	maybeAdd := func(si, ei int) {
		if a, ok := ascentFromIndices(smoothed, distances, si, ei); ok {
			candidates = append(candidates, a)
		}
	}

	for i := 1; i < len(smoothed); i++ {
		climbing := smoothed[i] > smoothed[i-1]
		if climbing && !inAscent {
			startIdx = i - 1
			inAscent = true
		} else if !climbing && inAscent {
			maybeAdd(startIdx, i-1)
			inAscent = false
		}
	}
	if inAscent {
		maybeAdd(startIdx, len(smoothed)-1)
	}
	return candidates
}

func ascentFromIndices(smoothed, distances []float64, si, ei int) (ascent, bool) {
	if ei <= si || si < 0 || ei >= len(smoothed) || len(smoothed) != len(distances) {
		return ascent{}, false
	}
	elevGain := smoothed[ei] - smoothed[si]
	lengthM := distances[ei] - distances[si]
	if elevGain <= 0 || lengthM <= 0 {
		return ascent{}, false
	}
	avgGrad := elevGain / lengthM * 100
	maxGrad := maxGradientPct(smoothed, distances, si, ei)
	return ascent{
		startIdx:      si,
		endIdx:        ei,
		startDistM:    distances[si],
		endDistM:      distances[ei],
		lengthM:       lengthM,
		elevationGain: elevGain,
		avgGradient:   math.Round(avgGrad*10) / 10,
		maxGradient:   math.Round(maxGrad*10) / 10,
	}, true
}

func mergeAndFilterAscentCandidates(candidates []ascent, smoothed, distances []float64, minGradient, minElevation float64) []ascent {
	if len(candidates) == 0 {
		return nil
	}

	merged := make([]ascent, 0, len(candidates))
	var active ascent
	activeSet := false

	flushActive := func() {
		if activeSet && (active.strongCore || qualifiesAscent(active, minGradient, minElevation)) {
			merged = append(merged, active)
		}
		activeSet = false
	}

	for _, curr := range candidates {
		curr.strongCore = qualifiesAscent(curr, minGradient, minElevation)
		if curr.strongCore {
			curr.coreEndIdx = curr.endIdx
			curr.coreEndDistM = curr.endDistM
		}
		currUsable := curr.strongCore || isAscentExtensionCandidate(curr, minGradient, minElevation)
		if !activeSet {
			if curr.strongCore {
				active = curr
				activeSet = true
			}
			continue
		}

		if !currUsable {
			continue
		}

		if shouldMergeAscentCandidate(active, curr, smoothed, distances, minGradient, minElevation) {
			mergedCandidate, ok := ascentFromIndices(smoothed, distances, active.startIdx, curr.endIdx)
			if !ok {
				flushActive()
				if curr.strongCore {
					active = curr
					activeSet = true
				}
				continue
			}
			mergedCandidate.strongCore = active.strongCore || curr.strongCore
			mergedCandidate.coreEndIdx = active.coreEndIdx
			mergedCandidate.coreEndDistM = active.coreEndDistM
			if curr.strongCore {
				mergedCandidate.coreEndIdx = curr.coreEndIdx
				mergedCandidate.coreEndDistM = curr.coreEndDistM
			}
			active = mergedCandidate
			continue
		}

		flushActive()
		if curr.strongCore {
			active = curr
			activeSet = true
		}
	}
	flushActive()
	return merged
}

func qualifiesAscent(a ascent, minGradient, minElevation float64) bool {
	return a.elevationGain >= minElevation &&
		a.lengthM >= ascentMinLengthM &&
		rawAvgGradientPct(a) >= minGradient
}

func isAscentExtensionCandidate(a ascent, minGradient, minElevation float64) bool {
	return a.elevationGain >= minElevation*ascentExtensionMinElevationFactor &&
		a.lengthM >= ascentMinLengthM &&
		rawAvgGradientPct(a) >= minGradient*ascentExtensionMinGradientFactor
}

func rawAvgGradientPct(a ascent) float64 {
	if a.lengthM <= 0 {
		return 0
	}
	return a.elevationGain / a.lengthM * 100
}

func shouldMergeAscentCandidate(prev, curr ascent, smoothed, distances []float64, minGradient, minElevation float64) bool {
	gapStartDist := prev.endDistM
	gapStartIdx := prev.endIdx
	if curr.strongCore && prev.coreEndDistM > 0 {
		gapStartDist = prev.coreEndDistM
		gapStartIdx = prev.coreEndIdx
	}

	gapDist := curr.startDistM - gapStartDist
	if gapDist < 0 || gapDist > ascentMergeMaxGapM {
		return false
	}
	if !prev.strongCore {
		return false
	}
	if !curr.strongCore && !isAscentExtensionCandidate(curr, minGradient, minElevation) {
		return false
	}

	dipElev := ascentDipElevation(gapStartIdx, curr.startIdx, smoothed)
	return dipElev <= ascentMergeDipTolerance(prev, curr)
}

func ascentDipElevation(prevEndIdx, currStartIdx int, smoothed []float64) float64 {
	if prevEndIdx < 0 || currStartIdx < prevEndIdx || prevEndIdx >= len(smoothed) {
		return math.Inf(1)
	}
	if currStartIdx >= len(smoothed) {
		currStartIdx = len(smoothed) - 1
	}

	endElev := smoothed[prevEndIdx]
	minElev := endElev
	for j := prevEndIdx; j <= currStartIdx; j++ {
		if smoothed[j] < minElev {
			minElev = smoothed[j]
		}
	}
	return endElev - minElev
}

func ascentMergeDipTolerance(prev, curr ascent) float64 {
	relativeBase := min(prev.elevationGain, curr.elevationGain)
	relativeTolerance := relativeBase * ascentMergeRelativeDipFactor
	return max(ascentMergeMinDipM, min(ascentMergeRelativeDipCapM, relativeTolerance))
}

func maxGradientPct(smoothed, distances []float64, si, ei int) float64 {
	maxGrad := 0.0
	if ei <= si || si < 0 || ei >= len(smoothed) || len(smoothed) != len(distances) {
		return maxGrad
	}

	totalDist := distances[ei] - distances[si]
	if totalDist <= 0 {
		return maxGrad
	}
	windowM := min(ascentMaxGradientWindowM, totalDist)
	for i := si; i < ei; i++ {
		targetDist := distances[i] + windowM
		if targetDist > distances[ei] {
			continue
		}
		targetElev, ok := interpolateElevationAtDistance(smoothed, distances, i, ei, targetDist)
		if !ok {
			continue
		}
		g := (targetElev - smoothed[i]) / windowM * 100
		g = min(g, 200.0) // sanity cap: alpine Steilstufen bis ~150% real
		if g > maxGrad {
			maxGrad = g
		}
	}
	return maxGrad
}

func interpolateElevationAtDistance(smoothed, distances []float64, si, ei int, targetDist float64) (float64, bool) {
	if targetDist < distances[si] || targetDist > distances[ei] {
		return 0, false
	}
	for i := si; i < ei; i++ {
		if distances[i+1] < targetDist {
			continue
		}
		segDist := distances[i+1] - distances[i]
		if segDist <= 0 {
			return smoothed[i], true
		}
		t := (targetDist - distances[i]) / segDist
		return smoothed[i]*(1-t) + smoothed[i+1]*t, true
	}
	return smoothed[ei], true
}

// classifyClimb returns a standard cycling climb category (HC, 1–4) based on
// elevation gain (meters) and average gradient (percent).
// Stored as text in activity_climbs.category for all sports. Consumers that
// numerically aggregate the column (gear.go climb_category_max) must filter
// HC out of the cast — HC presence is exposed separately as has_hc_climb.
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
