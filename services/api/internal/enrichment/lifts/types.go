package lifts

import (
	"errors"
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
// and AscentM == 0; an upward gondola the inverse; a bidirectional ride like
// the bidirectional aerial tramway yields both > 0).
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
	MaxSpeedCV           float64 // 0.05 — Coefficient of Variation, lifts run very stable
	SpeedCVTrimSeconds   int     // 15   — trim leading/trailing seconds before CV (handles lift accel/brake ramp)
	MinGradient          float64 // 0.20 — vertical/horizontal speed ratio
	MaxSinuosity         float64 // 1.15 — straight-line ratio
	MinDurationS         int     // 60   — fast lift floor
	NoSpeedMinAltRateMs  float64 // 0.6  — fallback when speed unusable
	NoSpeedMinDurationS  int     // 90
	BidirAltAbsM         float64 // 8.0  — bidirectional aerial tramway case: sum of |delta_alt| over window
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

	// Confirm-gating: when true, recomputeLiftStats() in the API layer counts
	// heuristic candidates regardless of confirmed-flag. Default false (current
	// product decision: heuristic stats only count after user confirm). Lives
	// in this struct so the algorithm tests can compile-time-check the flag,
	// but the actual gate sits in the SQL of recomputeLiftStats — see Phase 5b.
	AutoConfirmHeuristic bool // false
}

// DefaultDetectorConfig returns the calibrated defaults.
//
// Lift speeds are a property of the cable infrastructure (Schlepplift ≈ 1 m/s,
// Sessellift 0.85–4 m/s, Gondel/Pendelbahn 4–8 m/s) — they do NOT depend on
// the rider's discipline. The sport argument is therefore semantically
// incorrect as a speed-floor input, but it is currently used as a calibration
// fence: ski activities use a tighter floor (2.0 m/s) because casual walking
// on a straight uphill road otherwise passes all heuristic gates and creates
// false positives in the absence of OSM lift matches. mtb_downhill uses a
// lower floor (0.7 m/s) to pick up Bikepark Schlepplifte. Once the
// gradient/sinuosity/CV gates are tight enough to dismiss walking on their
// own, this branch should disappear.
func DefaultDetectorConfig(sport string) DetectorConfig {
	cfg := DetectorConfig{
		MinSpeedMs:               2.0,
		MaxSpeedMs:               12.0,
		MaxSpeedCV:               0.10,
		SpeedCVTrimSeconds:       15,
		MinGradient:              0.15,
		MaxSinuosity:             1.15,
		MinDurationS:             60,
		NoSpeedMinAltRateMs:      0.6,
		NoSpeedMinDurationS:      90,
		BidirAltAbsM:             8.0,
		BidirSinuosityMax:        1.10,
		BidirMinDurationS:        90,
		StopToleranceS:           120,
		SmartRecMaxGapS:          75,
		MinSamples:               4,
		WindowDurationS:          30,
		GPSDeltaSpeedFloorMs:     0.5,
		BoundarySpeedDropEnabled: false,
		BoundarySpeedDropMs:      1.5,
		BoundarySpeedDropWindowS: 60,
		AutoConfirmHeuristic:     false,
	}
	if sport == "mtb_downhill" || sport == "snowboarding" {
		// Bikepark / downhill MTB activity — Schlepplifte (T-bar drag-lift case)
		// and slow Sessellifte (slow-cable bikepark case's slowest sessel ≈ 0.85 m/s).
		// Walking at bikeparks is virtually nil so the calibration fence
		// at 0.7 m/s does not surface walking false positives in this
		// dataset.
		cfg.MinSpeedMs = 0.7
	}
	return cfg
}
