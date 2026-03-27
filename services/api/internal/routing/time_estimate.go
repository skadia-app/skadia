// Package routing provides utilities for route planning and time estimation.
package routing

import (
	"context"
	"log/slog"
	"math"
)

// UserSpeedFactorFetcher is implemented by the database layer to retrieve a
// user's historical speed factor relative to the Tobler baseline for a given
// sport type.
type UserSpeedFactorFetcher interface {
	GetUserSpeedFactor(ctx context.Context, userID string, sportType string) (float64, error)
}

// SACScale represents Swiss Alpine Club trail difficulty ratings.
// Multipliers < 1.0 slow the hiker down relative to the Tobler baseline.
type SACScale string

const (
	SACScaleT1 SACScale = "T1" // Hiking — paved/well-marked paths (baseline)
	SACScaleT2 SACScale = "T2" // Mountain hiking — unpaved paths, some exposure
	SACScaleT3 SACScale = "T3" // Demanding mountain hiking — scrambling sections
	SACScaleT4 SACScale = "T4" // Alpine hiking — hands sometimes needed
	SACScaleT5 SACScale = "T5" // Demanding alpine hiking — advanced scrambling
	SACScaleT6 SACScale = "T6" // Difficult alpine hiking — climbing-grade terrain
)

// sacMultiplier maps each SAC scale to a speed multiplier.
// Derived from empirical field data and SACScale research.
var sacMultiplier = map[SACScale]float64{
	SACScaleT1: 1.00,
	SACScaleT2: 0.90,
	SACScaleT3: 0.75,
	SACScaleT4: 0.60,
	SACScaleT5: 0.45,
	SACScaleT6: 0.30,
}

// Surface describes the type of trail or road surface.
type Surface string

const (
	SurfacePaved     Surface = "paved"
	SurfaceAsphalt   Surface = "asphalt"
	SurfaceConcrete  Surface = "concrete"
	SurfaceGravel    Surface = "gravel"
	SurfaceCompacted Surface = "compacted"
	SurfaceDirt      Surface = "dirt"
	SurfaceGrass     Surface = "grass"
	SurfaceSand      Surface = "sand"
	SurfaceRock      Surface = "rock"
	SurfaceScree     Surface = "scree"
	SurfaceSnow      Surface = "snow"
	SurfaceIce       Surface = "ice"
	SurfaceUnknown   Surface = ""
)

// surfaceMultiplier maps surface types to speed multipliers.
// Paved surfaces are baseline (1.0); loose or difficult surfaces reduce speed.
var surfaceMultiplier = map[Surface]float64{
	SurfacePaved:     1.00,
	SurfaceAsphalt:   1.00,
	SurfaceConcrete:  1.00,
	SurfaceGravel:    0.90,
	SurfaceCompacted: 0.95,
	SurfaceDirt:      0.85,
	SurfaceGrass:     0.80,
	SurfaceSand:      0.65,
	SurfaceRock:      0.70,
	SurfaceScree:     0.55,
	SurfaceSnow:      0.60,
	SurfaceIce:       0.45,
	SurfaceUnknown:   0.90, // conservative fallback
}

// ToblerSpeed computes hiking speed in km/h using the Tobler hiking function.
//
// Formula (E9-corrected):
//
//	speed = 6.0 * exp(-3.5 * |slope + 0.05|)
//
// slope is the gradient as a decimal (e.g. 0.15 for a 15% incline).
// The +0.05 offset accounts for the natural forward lean of the human gait:
// the optimum speed is reached at a slight downhill grade (~−5%).
func ToblerSpeed(slope float64) float64 {
	return 6.0 * math.Exp(-3.5*math.Abs(slope+0.05))
}

// SegmentInput describes a single route segment used as input for time estimation.
type SegmentInput struct {
	// DistanceM is the horizontal distance of the segment in metres.
	DistanceM float64
	// ElevationGain is the total ascent of the segment in metres.
	ElevationGain float64
	// ElevationLoss is the total descent of the segment in metres (positive value).
	ElevationLoss float64
	// SAC is the Swiss Alpine Club trail difficulty; leave empty for SACScaleT1 behaviour.
	SAC SACScale
	// Surface is the trail/road surface type.
	Surface Surface
}

// slope returns the net gradient as a decimal for the segment.
// Positive values indicate a net ascent; the caller is responsible for passing
// the correct sign based on direction of travel.
func (s SegmentInput) slope() float64 {
	if s.DistanceM <= 0 {
		return 0
	}
	// Use net elevation change (gain − loss) over horizontal distance.
	return (s.ElevationGain - s.ElevationLoss) / s.DistanceM
}

// EstimateParams holds all parameters for route-level time estimation.
type EstimateParams struct {
	// Segments is the ordered list of route segments to estimate.
	Segments []SegmentInput
	// SportType is the Garmin FIT SDK sport type string (e.g. "hiking", "running").
	// Used to look up the user's historical speed factor.
	SportType string
	// UserSpeedFactor is the calibration multiplier derived from the user's
	// recent activities. 1.0 means the user matches the Tobler baseline exactly.
	// Values > 1.0 mean the user is faster; values < 1.0 mean slower.
	// If zero the baseline (1.0) is used.
	UserSpeedFactor float64
}

// EstimateResult contains the time estimation output.
type EstimateResult struct {
	// TotalTimeMin is the estimated travel time in whole minutes.
	TotalTimeMin int
	// ToblerTimeMin is the uncalibrated Tobler time (before fitness adjustment) in minutes.
	ToblerTimeMin float64
	// UserSpeedFactor is the factor that was applied.
	UserSpeedFactor float64
}

// Estimate computes a fitness-calibrated time estimate for a multi-segment route.
//
// Algorithm:
//  1. For each segment, compute the Tobler speed from the net slope.
//  2. Apply the SAC-scale multiplier (trail difficulty).
//  3. Apply the surface multiplier.
//  4. Divide the segment distance by the adjusted speed to obtain segment time.
//  5. Sum all segment times.
//  6. Apply the user speed factor (fitness calibration).
//
// The result is rounded to the nearest whole minute.
func Estimate(params EstimateParams) EstimateResult {
	factor := params.UserSpeedFactor
	if factor <= 0 {
		factor = 1.0
	}

	var totalMinutes float64
	for _, seg := range params.Segments {
		if seg.DistanceM <= 0 {
			continue
		}

		// Step 1: Tobler base speed (km/h) from net slope.
		slope := seg.slope()
		speedKmh := ToblerSpeed(slope)

		// Step 2: SAC-scale multiplier.
		if m, ok := sacMultiplier[seg.SAC]; ok && seg.SAC != "" {
			speedKmh *= m
		}

		// Step 3: Surface multiplier.
		if m, ok := surfaceMultiplier[seg.Surface]; ok {
			speedKmh *= m
		} else {
			// Unknown surface: apply conservative fallback.
			speedKmh *= surfaceMultiplier[SurfaceUnknown]
		}

		// Guard against degenerate speeds after multipliers.
		if speedKmh < 0.1 {
			speedKmh = 0.1
		}

		// Step 4: segment time in minutes.
		distKm := seg.DistanceM / 1000.0
		segMinutes := (distKm / speedKmh) * 60.0
		totalMinutes += segMinutes
	}

	toblerTime := totalMinutes

	// Step 6: fitness calibration — divide by user factor so that a faster
	// user (factor > 1) yields a shorter estimated time.
	if factor > 0 {
		totalMinutes /= factor
	}

	return EstimateResult{
		TotalTimeMin:    int(math.Round(totalMinutes)),
		ToblerTimeMin:   math.Round(toblerTime*10) / 10,
		UserSpeedFactor: factor,
	}
}

// EstimateWithDB is a convenience wrapper that fetches the user's speed factor
// from the database and then calls Estimate.
//
// If the database lookup fails the function logs a warning and proceeds with
// factor 1.0 (graceful degradation — speed factor is optional calibration).
func EstimateWithDB(
	ctx context.Context,
	db UserSpeedFactorFetcher,
	userID string,
	params EstimateParams,
) EstimateResult {
	factor, err := db.GetUserSpeedFactor(ctx, userID, params.SportType)
	if err != nil {
		slog.Warn("could not fetch user speed factor, using baseline",
			"user_id", userID,
			"sport_type", params.SportType,
			"error", err,
		)
		factor = 1.0
	}
	params.UserSpeedFactor = factor
	return Estimate(params)
}
