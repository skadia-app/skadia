package activityutil

import (
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

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

// candidate holds intermediate data for a flagged point during GPS correction.
type candidate struct {
	index     int
	detour    float64
	isolation float64
	distM     float64
}

// CorrectGPSOutliers detects and interpolates isolated GPS spikes in a stream.
// It modifies points in-place: corrected points get their original coordinates
// stored in RawLat/RawLng and their position replaced by the midpoint of their
// neighbors. SpeedMS and DistanceM are cleared for corrected points (to be
// recomputed by FillMissingDistanceAndSpeed afterwards).
//
// The algorithm uses four passes:
//  1. Compute detour scores for all interior points with GPS.
//  2. Identify candidates exceeding MinDetourScore and MinDistanceM.
//  3. Filter by isolation ratio and consecutive-spike limit.
//  4. Apply interpolation in deterministic index order.
func CorrectGPSOutliers(points []model.StreamPoint, cfg CorrectionConfig) ([]model.StreamPoint, CorrectionReport) {
	report := CorrectionReport{TotalPoints: len(points)}

	if len(points) < cfg.MinPointsRequired {
		return points, report
	}

	n := len(points)

	// ── Pass 1: Compute detour scores ───────────────────────────────────
	scores := make([]float64, n)
	for i := 1; i < n-1; i++ {
		if points[i].Lat == nil || points[i].Lng == nil {
			continue
		}
		if points[i-1].Lat == nil || points[i-1].Lng == nil {
			continue
		}
		if points[i+1].Lat == nil || points[i+1].Lng == nil {
			continue
		}
		dPrev := HaversineMeters(*points[i-1].Lat, *points[i-1].Lng, *points[i].Lat, *points[i].Lng)
		dNext := HaversineMeters(*points[i].Lat, *points[i].Lng, *points[i+1].Lat, *points[i+1].Lng)
		dSkip := HaversineMeters(*points[i-1].Lat, *points[i-1].Lng, *points[i+1].Lat, *points[i+1].Lng)
		if dSkip < 0.1 {
			dSkip = 0.1
		}
		scores[i] = (dPrev + dNext) / dSkip
	}

	// ── Pass 2: Identify candidates ─────────────────────────────────────
	var candidates []candidate
	for i := 1; i < n-1; i++ {
		if scores[i] < cfg.MinDetourScore {
			continue
		}
		if points[i].Lat == nil || points[i].Lng == nil {
			continue
		}
		if points[i-1].Lat == nil || points[i-1].Lng == nil {
			continue
		}
		if points[i+1].Lat == nil || points[i+1].Lng == nil {
			continue
		}
		dPrev := HaversineMeters(*points[i-1].Lat, *points[i-1].Lng, *points[i].Lat, *points[i].Lng)
		dNext := HaversineMeters(*points[i].Lat, *points[i].Lng, *points[i+1].Lat, *points[i+1].Lng)
		maxDist := dPrev
		if dNext > maxDist {
			maxDist = dNext
		}
		if maxDist < cfg.MinDistanceM {
			continue
		}
		// Compute isolation: ratio of this point's detour to max neighbor detour.
		neighborMax := scores[i-1]
		if scores[i+1] > neighborMax {
			neighborMax = scores[i+1]
		}
		if neighborMax < 0.1 {
			neighborMax = 0.1
		}
		iso := scores[i] / neighborMax

		candidates = append(candidates, candidate{
			index:     i,
			detour:    scores[i],
			isolation: iso,
			distM:     maxDist,
		})
	}

	report.CandidateCount = len(candidates)

	// ── Pass 3: Filter by isolation and consecutive-spike limit ─────────
	toCorrect := make(map[int]candidate, len(candidates))
	for _, c := range candidates {
		if c.isolation < cfg.MinIsolation {
			report.SkippedIsolation++
			report.Points = append(report.Points, PointCorrection{
				Index:       c.index,
				DetourScore: c.detour,
				Isolation:   c.isolation,
				DistanceM:   c.distM,
				Corrected:   false,
				OriginalLat: *points[c.index].Lat,
				OriginalLng: *points[c.index].Lng,
			})
			continue
		}
		toCorrect[c.index] = c
	}

	// Find consecutive runs and remove those exceeding MaxConsecutiveSpikes.
	if len(toCorrect) > 0 {
		indices := make([]int, 0, len(toCorrect))
		for idx := range toCorrect {
			indices = append(indices, idx)
		}
		sort.Ints(indices)

		// Identify runs of consecutive indices.
		runStart := 0
		for i := 1; i <= len(indices); i++ {
			if i < len(indices) && indices[i] == indices[i-1]+1 {
				continue
			}
			// End of run: indices[runStart..i-1]
			runLen := i - runStart
			if runLen > cfg.MaxConsecutiveSpikes {
				for j := runStart; j < i; j++ {
					c := toCorrect[indices[j]]
					report.SkippedConsec++
					report.Points = append(report.Points, PointCorrection{
						Index:       c.index,
						DetourScore: c.detour,
						Isolation:   c.isolation,
						DistanceM:   c.distM,
						Corrected:   false,
						OriginalLat: *points[c.index].Lat,
						OriginalLng: *points[c.index].Lng,
					})
					delete(toCorrect, indices[j])
				}
			}
			runStart = i
		}
	}

	// ── Pass 4: Apply corrections in deterministic order ────────────────
	corrIndices := make([]int, 0, len(toCorrect))
	for idx := range toCorrect {
		corrIndices = append(corrIndices, idx)
	}
	sort.Ints(corrIndices)

	for _, idx := range corrIndices {
		c := toCorrect[idx]

		// Store original coordinates.
		origLat := *points[idx].Lat
		origLng := *points[idx].Lng
		points[idx].RawLat = &origLat
		points[idx].RawLng = &origLng

		// Interpolate position as midpoint of neighbors.
		midLat := (*points[idx-1].Lat + *points[idx+1].Lat) / 2
		midLng := (*points[idx-1].Lng + *points[idx+1].Lng) / 2
		points[idx].Lat = &midLat
		points[idx].Lng = &midLng

		// Clear derived fields — FillMissingDistanceAndSpeed will recompute.
		points[idx].SpeedMS = nil
		points[idx].DistanceM = nil

		report.CorrectedCount++
		report.Points = append(report.Points, PointCorrection{
			Index:       c.index,
			DetourScore: c.detour,
			Isolation:   c.isolation,
			DistanceM:   c.distM,
			Corrected:   true,
			OriginalLat: origLat,
			OriginalLng: origLng,
		})
	}

	return points, report
}

// SpeedCorrectionConfig holds tunable parameters for gradient-aware speed filtering.
type SpeedCorrectionConfig struct {
	// MaxSpeedByGroup maps segment group (bike/foot/ski/paddle/swim/generic) to the
	// base max speed in m/s on flat ground. Gradient bonus is added on top.
	// These are generous — a well-trained road cyclist can sprint to 65 km/h on flat.
	MaxSpeedByGroup map[string]float64

	// GradientBonusFactor: for each 1% downhill gradient, add this many m/s
	// to the base max speed. Default: 1.4 (≈ 5 km/h per 1% gradient).
	// At -10%: bonus = 1.4 * 10 = 14 m/s (50 km/h).
	GradientBonusFactor float64

	// MinOvershootRatio: speed must exceed max_speed * this ratio to be flagged.
	// Prevents false positives from minor GPS jitter near the threshold.
	// Default: 1.3 (speed must be 30% above max to trigger).
	MinOvershootRatio float64

	// CadenceBoostFactor: if cadence > 0 at a spike point, multiply the
	// allowed max speed by this factor. Rider is actively pedaling/running.
	// Default: 1.3
	CadenceBoostFactor float64

	// MaxAccelMS2: maximum plausible acceleration in m/s².
	// Currently informational only — recorded in report for future tuning.
	// Reserved for a future acceleration-based filter pass.
	// Default: 5.0
	MaxAccelMS2 float64

	// MaxConsecutiveOver: if more than N consecutive points all exceed the
	// threshold, treat as real movement (gondola, shuttle), not GPS error.
	// Default: 3
	MaxConsecutiveOver int

	// MinPointsRequired: minimum stream length to attempt correction.
	// Default: 5
	MinPointsRequired int
}

// DefaultSpeedCorrectionConfig returns conservative defaults for speed filtering.
func DefaultSpeedCorrectionConfig() SpeedCorrectionConfig {
	return SpeedCorrectionConfig{
		MaxSpeedByGroup: map[string]float64{
			"bike":    19.4, // 70 km/h flat — road sprint, drafting, any bike type
			"foot":     8.3, // 30 km/h flat — fast trail running downhill
			"ski":     22.2, // 80 km/h flat — ski alpin legitimately fast
			"paddle":   5.6, // 20 km/h
			"swim":     2.8, // 10 km/h
			"generic": 19.4, // conservative default (= bike)
		},
		GradientBonusFactor: 1.4,
		MinOvershootRatio:   1.3,
		CadenceBoostFactor:  1.3,
		MaxAccelMS2:         5.0,
		MaxConsecutiveOver:  3,
		MinPointsRequired:   5,
	}
}

// SpeedPointCorrection records the speed filter decision for a single point.
type SpeedPointCorrection struct {
	Index        int     // stream index
	SpeedMS      float64 // GPS-derived speed in m/s
	MaxAllowedMS float64 // computed max speed (with gradient + cadence boosts)
	GradientPct  float64 // gradient at this point (negative = downhill)
	HasCadence   bool    // cadence > 0 at this point
	AccelMS2     float64 // acceleration from previous point (informational)
	Corrected    bool    // true if position was interpolated
}

// SpeedCorrectionReport summarizes speed-based corrections.
type SpeedCorrectionReport struct {
	TotalPoints    int                    // total stream points
	CandidateCount int                    // points exceeding max speed * overshoot ratio
	CorrectedCount int                    // actually corrected
	SkippedConsec  int                    // skipped because consecutive > max
	Points         []SpeedPointCorrection // per-point detail
}
