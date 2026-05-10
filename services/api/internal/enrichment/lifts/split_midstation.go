package lifts

// Multi-station-cable splitter — addresses SyntheticCableTraverse corpus ride 10 where
// a single bidir candidate covers two physical cable segments separated by
// an intermediate station. The mid-station signature is recognisable from
// the stream alone (no GT lookup): a sustained low-speed alt-plateau in
// the geographic middle of the candidate, surrounded by cable-class flow
// on either side. When found, the candidate splits into two sub-rides and
// each is rebuilt from computeStats so downstream metrics stay consistent.

import "time"

const (
	// midStationMinPlateauS — minimum window length where speed and alt are
	// both station-flat. Real SyntheticCableTraverse mid-station-2 plateau lasts 30 s in
	// the recorded stream; shorter windows indicate a pylon-stop or GPS
	// blip and must not split.
	midStationMinPlateauS = 30.0
	// midStationMaxPlateauS — beyond this the gap is more likely a true
	// disembark / ride-end than an intermediate station; defer to the
	// upstream end-of-ride detection rather than splitting here.
	midStationMaxPlateauS = 120.0
	// midStationMaxSpeedMs — speed cap for the plateau window. Real cabin
	// hold during station passage measures 0.3–1.0 m/s; 1.5 m/s leaves
	// headroom for GPS jitter at low speeds without admitting cable-class
	// flow (typically 2.0–4.5 m/s on the corpus segments).
	midStationMaxSpeedMs = 1.5
	// midStationMaxAltDriftM — maximum alt change inside the plateau. The
	// platform floor is flat; real plateau alt drifts 0–4 m. 5 m allows
	// for pressure-sensor jitter without admitting a real climb segment.
	midStationMaxAltDriftM = 5.0
	// midStationEdgeMarginS — the plateau midpoint must sit at least this
	// many seconds from both the candidate's start and end. Without this
	// gate a station signature in the very first or last 30 s of a ride
	// would split off a degenerate sub-ride below the validate threshold.
	midStationEdgeMarginS = 60.0
	// midStationMinSubRideS — minimum length each sub-ride must reach for
	// the split to commit. 120 s separates the SyntheticCableTraverse seg2/seg3 cables
	// (240 s and 270 s long after split) from short saddle-bidir rides
	// such as SyntheticBikeparkB ride 7 (188 s total → ~90 s sub-rides if split,
	// rejected here). The single 60 s floor was over-aggressive on the
	// short-saddle case.
	midStationMinSubRideS = 120.0
	// midStationMinSubRideAltMassM — minimum |altPos|+|altNeg| for each
	// sub-ride. Adds a "substance" check beyond pure duration: a rider
	// who briefly waits mid-cable for an attendant signal still has near-
	// zero alt change in their before/after windows; flags those out.
	midStationMinSubRideAltMassM = 60.0
)

// splitAtMidStations scans bidir candidates for an in-window mid-station
// plateau and splits them into two sub-rides at that boundary. Non-bidir
// rides pass through unchanged — high/medium candidates have monotonic
// profiles and never cross intermediate stations as a single window.
//
// Each split sub-ride is rebuilt via computeStats so AscentM, DescentM,
// DistanceM, and DurationS reflect the actual sub-window, not the parent's
// totals. Confidence is inherited from the parent (bidir); a future pass
// can re-classify if needed.
func splitAtMidStations(in []rideWithIdx, rows []StreamPoint, cfg DetectorConfig) []rideWithIdx {
	out := make([]rideWithIdx, 0, len(in)+2)
	for _, r := range in {
		if r.ride.Confidence != "bidir" {
			out = append(out, r)
			continue
		}
		plateauStart, plateauEnd, ok := findMidStationPlateau(rows, r.startIdx, r.endIdx)
		if !ok {
			out = append(out, r)
			continue
		}
		subAEnd := plateauStart - 1
		subBStart := plateauEnd + 1
		if subAEnd <= r.startIdx || subBStart >= r.endIdx {
			out = append(out, r)
			continue
		}
		subADurS := rows[subAEnd].Time.Sub(rows[r.startIdx].Time).Seconds()
		subBDurS := rows[r.endIdx].Time.Sub(rows[subBStart].Time).Seconds()
		if subADurS < midStationMinSubRideS || subBDurS < midStationMinSubRideS {
			out = append(out, r)
			continue
		}
		subA := buildSubRide(rows, r.startIdx, subAEnd, r.ride.Confidence, cfg)
		subB := buildSubRide(rows, subBStart, r.endIdx, r.ride.Confidence, cfg)
		// Defense-in-depth: each sub-ride must carry enough alt mass to
		// stand on its own. A sub-ride below the floor would have been
		// rejected by the upstream lift-mass gate had it been detected
		// directly; preserving the parent intact is the safer outcome.
		if subA.ride.AscentM+subA.ride.DescentM < midStationMinSubRideAltMassM ||
			subB.ride.AscentM+subB.ride.DescentM < midStationMinSubRideAltMassM {
			out = append(out, r)
			continue
		}
		out = append(out, subA, subB)
	}
	return out
}

// findMidStationPlateau locates the longest sustained plateau window
// inside [startIdx, endIdx] whose midpoint sits at least
// midStationEdgeMarginS from both ends of the candidate ride. Returns
// (plateauStartIdx, plateauEndIdx, true) when a qualifying window exists.
func findMidStationPlateau(rows []StreamPoint, startIdx, endIdx int) (int, int, bool) {
	if endIdx-startIdx < 4 {
		return 0, 0, false
	}
	rideStartT := rows[startIdx].Time
	rideEndT := rows[endIdx].Time
	rideDurS := rideEndT.Sub(rideStartT).Seconds()
	if rideDurS < 2*midStationEdgeMarginS+midStationMinPlateauS {
		return 0, 0, false
	}

	bestStart, bestEnd, bestLen := -1, -1, 0.0
	curStart := -1
	var curMinAlt, curMaxAlt float64
	flushPlateau := func(curEnd int) {
		if curStart < 0 || curEnd < curStart {
			return
		}
		ok, length := plateauQualifies(rows, curStart, curEnd, rideStartT, rideEndT)
		if ok && length > bestLen {
			bestStart, bestEnd, bestLen = curStart, curEnd, length
		}
	}
	for k := startIdx; k <= endIdx; k++ {
		p := rows[k]
		passes := samplePassesMidStationGate(p)
		if passes && p.AltitudeM != nil {
			a := *p.AltitudeM
			if curStart < 0 {
				curStart = k
				curMinAlt, curMaxAlt = a, a
			} else {
				if a < curMinAlt {
					curMinAlt = a
				}
				if a > curMaxAlt {
					curMaxAlt = a
				}
				if curMaxAlt-curMinAlt > midStationMaxAltDriftM {
					passes = false
				}
			}
		}
		if !passes {
			flushPlateau(k - 1)
			curStart = -1
		}
	}
	flushPlateau(endIdx)
	if bestStart < 0 {
		return 0, 0, false
	}
	return bestStart, bestEnd, true
}

func samplePassesMidStationGate(p StreamPoint) bool {
	if p.AltitudeM == nil {
		return false
	}
	if p.SpeedMs != nil && *p.SpeedMs > midStationMaxSpeedMs {
		return false
	}
	return true
}

// plateauQualifies checks the candidate plateau's length and edge margin
// against the parent ride's [rideStartT, rideEndT] bounds. Returns
// (qualifies, plateau-length-in-seconds).
func plateauQualifies(rows []StreamPoint, plateauStart, plateauEnd int, rideStartT, rideEndT time.Time) (bool, float64) {
	durS := rows[plateauEnd].Time.Sub(rows[plateauStart].Time).Seconds()
	if durS < midStationMinPlateauS || durS > midStationMaxPlateauS {
		return false, 0
	}
	plateauMidT := rows[plateauStart].Time.Add(time.Duration(durS/2) * time.Second)
	startMargin := plateauMidT.Sub(rideStartT).Seconds()
	endMargin := rideEndT.Sub(plateauMidT).Seconds()
	if startMargin < midStationEdgeMarginS || endMargin < midStationEdgeMarginS {
		return false, 0
	}
	return true, durS
}

// buildSubRide rebuilds a rideWithIdx for [startIdx, endIdx] from the raw
// stream by re-running computeStats. Inherits the parent's Confidence so
// matchers that condition on it stay consistent.
func buildSubRide(rows []StreamPoint, startIdx, endIdx int, confidence string, cfg DetectorConfig) rideWithIdx {
	st := computeStats(rows, startIdx, endIdx, cfg)
	return rideWithIdx{
		ride: LiftRide{
			StartedAt:  rows[startIdx].Time,
			EndedAt:    rows[endIdx].Time,
			DurationS:  int(rows[endIdx].Time.Sub(rows[startIdx].Time).Seconds()),
			AscentM:    st.altPosDelta,
			DescentM:   st.altNegDelta,
			DistanceM:  st.cumDist,
			Confidence: confidence,
		},
		startIdx: startIdx,
		endIdx:   endIdx,
	}
}
