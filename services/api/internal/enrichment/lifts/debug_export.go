package lifts

// Debug-only re-exports of internal helpers for the cmd/lift_detect_debug
// CLI. The package is otherwise opaque; these wrappers let the iteration
// harness inspect the gating decisions without changing internal API shape.

type DebugStats struct {
	DurationS   float64
	AltRateMs   float64
	AltPos      float64
	AltNeg      float64
	Sinuosity   float64
	MeanSpeed   float64
	SpeedCV     float64
	SpeedsCount int
	Speeds      []float64
	AltsOK      bool
}

func PublicBuildInitialWindow(rows []StreamPoint, i int, cfg DetectorConfig) (start, end int, ok bool) {
	return buildInitialWindow(rows, i, cfg)
}

func PublicComputeStats(rows []StreamPoint, start, end int, cfg DetectorConfig) DebugStats {
	s := computeStats(rows, start, end, cfg)
	return DebugStats{
		DurationS:   s.durationS,
		AltRateMs:   s.altRateMs,
		AltPos:      s.altPosDelta,
		AltNeg:      s.altNegDelta,
		Sinuosity:   s.sinuosity,
		MeanSpeed:   s.meanSpeed,
		SpeedCV:     s.speedCV,
		SpeedsCount: len(s.speeds),
		Speeds:      append([]float64(nil), s.speeds...),
		AltsOK:      s.altsOK,
	}
}

func PublicClassify(d DebugStats, cfg DetectorConfig) string {
	s := stats{
		durationS:   d.DurationS,
		altRateMs:   d.AltRateMs,
		altPosDelta: d.AltPos,
		altNegDelta: d.AltNeg,
		sinuosity:   d.Sinuosity,
		meanSpeed:   d.MeanSpeed,
		speedCV:     d.SpeedCV,
		speedsOK:    d.SpeedsCount > 0,
		altsOK:      d.AltsOK,
	}
	return classify(s, cfg)
}

// PublicTryRide simulates the full algorithm at one starting index i, returning
// the extended end index, the final stats, the kind classification, and whether
// validateRide accepted. Used by lift_detect_debug to understand which gate
// rejects in each iteration.
func PublicTryRide(rows []StreamPoint, i int, cfg DetectorConfig) (start, finalEnd int, kind string, fin DebugStats, valid bool, ok bool) {
	st, en, gotWin := buildInitialWindow(rows, i, cfg)
	if !gotWin {
		return 0, 0, "", DebugStats{}, false, false
	}
	ws := computeStats(rows, st, en, cfg)
	k := classify(ws, cfg)
	if k == "" {
		return st, en, "", PublicComputeStats(rows, st, en, cfg), false, true
	}
	fe := extendCandidate(rows, st, en, k, ws, cfg)
	finalStats := computeStats(rows, st, fe, cfg)
	valid = validateRide(finalStats, k, cfg)
	return st, fe, k, PublicComputeStats(rows, st, fe, cfg), valid, true
}
