package lifts

import (
	"errors"
	"strings"
	"time"
)

// StreamPoint is one sample of an activity stream as consumed by the lift
// detector. Pointer fields preserve the SQL NULL semantics of activity_streams.
// The detector treats nil as "no data" — never as zero.
type StreamPoint struct {
	Time      time.Time
	Lat       *float64
	Lng       *float64
	AltitudeM *float64
	SpeedMs   *float64
	DistanceM *float64
}

// LiftRide is one detected lift segment in the stream. Distance/ascent/descent
// are direction-agnostic totals (a downhill cable car contributes DescentM > 0
// and AscentM == 0; an upward gondola the inverse; a bidirectional aerial
// tramway crossing a saddle yields both > 0).
type LiftRide struct {
	StartedAt time.Time
	EndedAt   time.Time
	DurationS int
	AscentM   float64
	DescentM  float64
	DistanceM float64
	// Confidence levels: "high" (speed-stable + gradient-strict),
	// "medium" (no usable speed but altitude-rate strong), "low" (resume
	// gap inference; reserved for the pause-detector path).
	Confidence string
}

func (r LiftRide) Validate() error {
	if r.EndedAt.Before(r.StartedAt) {
		return errors.New("lifts: ride end before start")
	}
	return nil
}

// DetectorConfig holds tunable thresholds for the heuristic Method-B detector.
// The defaults are calibrated against the LIFT_ERKENNUNG training corpus
// (9 regions, 13 activities, ski + mtb_downhill).
type DetectorConfig struct {
	MinSpeedMs           float64 // 2.0  — fast lift floor
	MaxSpeedMs           float64 // 12.0 — fast lift ceiling
	MaxSpeedCV           float64 // 0.05 — Coefficient of Variation, lifts run very stable (final validate)
	InitialMaxSpeedCV    float64 // 0.10 — relaxed CV for the initial 30 s classify gate (3 chunks ≈ volatile MAD)
	SpeedCVTrimSeconds   int     // 30   — trim leading/trailing seconds before CV. Active to let backward-extension absorb 30–60 s of boarding-phase samples (queue-shuffle, pylon-acceleration) without exploding the strict 0.04 final-CV gate. Window must be > 2*trim+30 s for the trim to kick in, otherwise full speed series is used.
	MinGradient          float64 // 0.20 — vertical/horizontal speed ratio
	MaxSinuosity         float64 // 1.15 — straight-line ratio
	MinDurationS         int     // 60   — fast lift floor
	NoSpeedMinAltRateMs  float64 // 0.6  — fallback when speed unusable
	NoSpeedMinDurationS  int     // 90
	BidirAltAbsM         float64 // 8.0  — bidirectional aerial tramway: sum of |delta_alt| over window
	BidirSinuosityMax    float64 // 1.10 — bidir lifts MUST be very straight
	BidirMinDurationS    int     // 90  — bidir candidate min duration (FP-shield against MTB flow waves)
	StopToleranceS       int     // 120  — stop ≤ this preserves segment
	SmartRecMaxGapS      int     // 35   — gap above this counts as recording-paused
	MinSamples           int     // 10
	WindowDurationS      int     // 30 — initial window for the candidate test
	GPSDeltaSpeedFloorMs float64 // 0.5  — below this we ignore the SpeedMs field

	// BoundarySpeedDropEnabled: when true, a candidate ride must show a clear
	// speed drop on at least one boundary (mean speed in BoundarySpeedDropWindowS
	// preceding/following < BoundarySpeedDropMs). Default false.
	BoundarySpeedDropEnabled bool
	BoundarySpeedDropMs      float64 // 1.5  — walking pace floor
	BoundarySpeedDropWindowS int     // 60   — flanking window each side
	RoadClimbBoundaryGuard   bool    // cycling road-climb false-positive guard

	// Confirm-gating: when true, recomputeLiftStats() in the API layer counts
	// heuristic candidates regardless of confirmed-flag. Default false (current
	// product decision: heuristic stats only count after user confirm). Lives
	// in this struct so the algorithm tests can compile-time-check the flag,
	// but the actual gate sits in the SQL of recomputeLiftStats — see Phase 5b.
	AutoConfirmHeuristic bool // false
}

// DefaultDetectorConfig returns the calibrated defaults.
//
// Lift detection is mostly sport-agnostic: a cable-car ride looks the same to
// the stream regardless of whether the rider is skiing, biking, snowboarding,
// or hiking up to the lift on foot. The MinSpeedMs floor is therefore pinned
// to the slowest infrastructure class (T-bar / drag lift ≈ 0.7 m/s) so the
// same algorithm covers chairlifts (0.85–4 m/s), gondolas (4–8 m/s), and
// aerial tramways (up to 12 m/s). Plain cycling enables an extra road-climb
// boundary guard because steady paved climbs can otherwise mimic cable motion.
func DefaultDetectorConfig(sport string) DetectorConfig {
	roadClimbGuard := strings.EqualFold(sport, "cycling")
	return DetectorConfig{
		MinSpeedMs:               0.7,
		MaxSpeedMs:               15.0,
		MaxSpeedCV:               0.04,
		InitialMaxSpeedCV:        0.04,
		SpeedCVTrimSeconds:       30,
		MinGradient:              0.10,
		MaxSinuosity:             1.08,
		MinDurationS:             60,
		NoSpeedMinAltRateMs:      0.6,
		NoSpeedMinDurationS:      90,
		BidirAltAbsM:             50.0,
		BidirSinuosityMax:        1.05,
		BidirMinDurationS:        120,
		StopToleranceS:           120,
		SmartRecMaxGapS:          75,
		MinSamples:               4,
		WindowDurationS:          30,
		GPSDeltaSpeedFloorMs:     0.5,
		BoundarySpeedDropEnabled: false,
		BoundarySpeedDropMs:      1.5,
		BoundarySpeedDropWindowS: 60,
		RoadClimbBoundaryGuard:   roadClimbGuard,
		AutoConfirmHeuristic:     false,
	}
}
