package lifts

// Pure-descent reclassification — promotes bidir rides that look like a
// downhill gondola (the calibrated pure-descent corpus case: alt drop
// 511 m over 350 s, asc 0.3 m, sin ≈ 1.01, mean 4.4 m/s, CV ≈ 0.04) to
// "high" confidence so per-direction labelling reflects reality.
//
// The legacy classify pipeline only emits "high" for ascending windows
// because clean ski cruises share the per-window speed/sin/CV plane.
// At the FULL ride scale that ambiguity collapses: a gondola accumu-
// lates only GPS-noise ascent (< 1 % of mass) over a multi-minute run,
// while ski descents pick up 5–15 % through traverses or sit at sin
// ≥ 1.05 from carving turns. The post-hoc gate sees these final stats
// and is therefore tighter than any per-window check could be.

const (
	// reclassMinDescentM — a real downhill gondola covers ≥ 50 m of
	// vertical drop. Below that floor a "bidir" ride is more likely a
	// short connector or a saddle nudge that the bidir branch already
	// classifies correctly.
	reclassMinDescentM = 50.0
	// reclassMaxAscentRatio — ascent over total mass. Real cables sit
	// at ≤ 0.005 (GPS noise only); ski cruises pick up ≥ 0.05 through
	// traverses. The 0.01 cap keeps a 2× margin to the noisiest cable
	// runs without admitting any ski descent in the corpus.
	reclassMaxAscentRatio = 0.01
	// reclassMaxSinuosity — gondola catenaries hold sin ≤ 1.03 even
	// over GPS noise on the full cable span; ski cruises veer to
	// 1.05+ from S-turns or carving. Tighter than BidirSinuosityMax
	// (1.05) on purpose.
	reclassMaxSinuosity = 1.03
	// reclassMaxSpeedCV — mechanically fixed cables hold CV ≤ 0.04
	// even on slow chairlifts; ski-cruise CV is ≥ 0.10 on real
	// pistes. The cap matches the per-window MaxSpeedCV used by the
	// ascending "high" branch — a descending cable should clear the
	// same bar.
	reclassMaxSpeedCV = 0.04
	// reclassMinMeanSpeedMs — gondolas run at ≥ 2.5 m/s; below that
	// a "bidir" ride is more likely a slow walking / pushing segment
	// that happened to clear the sin gate.
	reclassMinMeanSpeedMs = 2.5
)

// reclassifyPureDescentRides walks the merged ride list, recomputes
// stats over each bidir ride's window, and promotes the ride to "high"
// when its full-window signature is unambiguously a downhill gondola.
// The slice is mutated in place; ride boundaries are not changed.
func reclassifyPureDescentRides(in []rideWithIdx, rows []StreamPoint, cfg DetectorConfig) []rideWithIdx {
	for k := range in {
		r := &in[k]
		if r.ride.Confidence != "bidir" {
			continue
		}
		mass := r.ride.AscentM + r.ride.DescentM
		if mass <= 0 {
			continue
		}
		if r.ride.DescentM < reclassMinDescentM {
			continue
		}
		// Ascent must be both small absolute (≤ 5 m, GPS noise floor)
		// and small ratio (≤ 1 % of mass) — either alone admits edge
		// cases (a 100 m descent with 4 m ascent passes the absolute
		// gate but is still 4 % saddle-dominated; a 1000 m descent
		// with 9 m ascent passes the ratio gate but the absolute is
		// already in ski-traverse territory).
		if r.ride.AscentM > 5.0 || r.ride.AscentM/mass > reclassMaxAscentRatio {
			continue
		}
		// Recompute window stats so sin / CV / mean reflect the final
		// ride boundaries (after merge / split). The per-ride stats
		// stored on r.ride don't carry sinuosity or CV.
		st := computeStats(rows, r.startIdx, r.endIdx, cfg)
		if !st.altsOK || !st.speedsOK {
			continue
		}
		if st.sinuosity > reclassMaxSinuosity {
			continue
		}
		if st.speedCV > reclassMaxSpeedCV {
			continue
		}
		if st.meanSpeed < reclassMinMeanSpeedMs {
			continue
		}
		// Downward direction sanity: altRateMs must be negative (the
		// "Pure descent" name is literal) — a saddle ride with net
		// near-zero altRate and balanced asc / desc is properly bidir.
		if st.altRateMs > -0.05 {
			continue
		}
		r.ride.Confidence = "high"
	}
	return in
}
