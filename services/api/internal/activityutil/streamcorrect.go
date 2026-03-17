package activityutil

// CorrectionConfig holds all tunable parameters for GPS stream correction.
// Every threshold is exposed so future training-data-based optimization
// can adjust them without code changes.
type CorrectionConfig struct {
	// MinDetourScore: points with detour score below this are never flagged.
	// detour = (d_prev + d_next) / d_skip. Straight line ≈ 1.0.
	// Default: 4.0 (conservative — only obvious spikes).
	MinDetourScore float64

	// MinIsolation: flagged points must have this ratio vs their neighbors'
	// detour scores. Prevents false positives on switchbacks/hairpins.
	// isolation = detour(i) / max(detour(i-1), detour(i+1))
	// Default: 3.0
	MinIsolation float64

	// MinDistanceM: ignore corrections for points that moved less than
	// this distance from neighbors. Prevents micro-corrections on standing GPS drift.
	// Default: 5.0 meters
	MinDistanceM float64

	// MaxConsecutiveSpikes: if more than N consecutive points are flagged,
	// treat as real trajectory change (GPS reacquisition, not individual spikes).
	// Default: 3
	MaxConsecutiveSpikes int

	// MinPointsRequired: minimum stream length to attempt correction.
	// Default: 5
	MinPointsRequired int
}

// DefaultCorrectionConfig returns conservative defaults that minimize
// false positives. Suitable for all sport types without tuning.
func DefaultCorrectionConfig() CorrectionConfig {
	return CorrectionConfig{
		MinDetourScore:       4.0,
		MinIsolation:         3.0,
		MinDistanceM:         5.0,
		MaxConsecutiveSpikes: 3,
		MinPointsRequired:    5,
	}
}

// PointCorrection records the decision for a single stream point.
// Used for evaluation against local evaluation fixtures.
type PointCorrection struct {
	Index        int     // index into the stream slice
	DetourScore  float64 // raw detour score
	Isolation    float64 // isolation ratio vs neighbors
	DistanceM    float64 // max(d_prev, d_next) in meters
	Corrected    bool    // true if position was interpolated
	OriginalLat  float64 // original latitude (before correction)
	OriginalLng  float64 // original longitude (before correction)
}

// CorrectionReport summarizes all corrections applied to a stream.
// Can be serialized to JSON for training-data evaluation.
type CorrectionReport struct {
	TotalPoints      int               // total stream points
	CandidateCount   int               // points exceeding MinDetourScore
	CorrectedCount   int               // actually corrected points
	SkippedConsec    int               // skipped because consecutive > max
	SkippedIsolation int               // skipped because isolation too low
	Points           []PointCorrection // per-point detail (only for candidates)
}
