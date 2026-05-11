package lifts

// Saddle-cable recovery — addresses the multi-station gondola corpus
// case where a return-trip mid-section cable (top-station → mid-station)
// is missed by the main forward walk because the cable's alt profile
// dips through a saddle below the boarding point. Total mass over the
// short fast-flat saddle (~160 s, asc ≈ 30 m, desc ≈ 12 m, mass ≈ 43 m,
// speedCV ≈ 0.17) sits below the 60 m floorMetric and the seed never
// fires.
//
// This pass scans the stream for unclaimed cable-class flow whose
// physical signature distinguishes it from skiing/biking/walking
// traverses across the same terrain, then anchors the candidate to
// known lift infrastructure in the same activity through a dual-
// position gate:
//
//   - speed mean ∈ [1.5, 7.0] m/s        — covers chairlift to detachable
//   - speedCV ≤ 0.20                     — mech.-cable rate (the
//                                          calibrated saddle-cable case
//                                          measured 0.168), with head-
//                                          room for GPS jitter
//   - sustained ≥ 90 s                   — filters short stops + GPS noise
//   - 25 m ≤ asc + desc ≤ 80 m            — saddle-bidir window, not real climb
//   - asc ≥ 5 m AND desc ≥ 5 m            — both directions present
//   - no synthetic recording-pause filler — guards against speed=0 spans
//   - dual position anchor               — both cable endpoints must sit
//                                          within 100 m of an existing
//                                          detected ride's endpoint
//   - sinuosity ≤ 1.20                   — gondola catenaries are nearly
//                                          straight; bikepark connector
//                                          loops at lift tops measure 5+
//
// The dual-anchor gate is the FP-guard that filters cycling/skiing
// activities: their cycling-cruise pattern can satisfy CV ≤ 0.20 over
// rolling roads, but they have zero detected lifts → no anchor positions
// → recovery rejects entirely. For the multi-station saddle case the
// missing return-trip's endpoints match the previous outbound's top
// station and the next return segment's mid station within ≈30 m each.
//
// Output rides are flagged confidence="bidir" so the downstream mid-
// station splitter can split them at intermediate stations if needed.

import (
	"math"
	"sort"
)

const (
	// saddleCableMinDurationS — lower duration bound. Below this, a stretch
	// of cable-class signal is more likely a brief lift glitch or a GPS
	// freeze rather than a real cable trip; the corpus never has a real
	// cable shorter than this.
	saddleCableMinDurationS = 90.0
	// saddleCableMinMassM / saddleCableMaxMassM — alt-mass band. Below
	// 25 m the candidate is a stationary platform plus baro noise; above
	// 80 m the upstream extender / floor would have admitted it as a real
	// lift candidate.
	saddleCableMinMassM = 25.0
	saddleCableMaxMassM = 80.0
	// saddleCableMinSpeedMs / saddleCableMaxSpeedMs — speed mean band
	// covering the cable spectrum from slow chairlift (≈1.5 m/s) to
	// detachable gondola (≈6.5 m/s). The 7.0 m/s upper edge keeps trains
	// and high-speed downhill skiing out.
	saddleCableMinSpeedMs = 1.5
	saddleCableMaxSpeedMs = 7.0
	// saddleCableMaxSpeedCV — coefficient of variation cap on speed.
	// The calibrated saddle-cable case measured CV 0.168 (range 2.22–
	// 7.90 m/s) — the cable accelerates/decelerates at towers and slows
	// over the saddle dip. The 0.20 cap fits real cable behaviour with
	// headroom; the dual-anchor gate is the actual FP-guard against
	// human-powered traverses with similar CV.
	saddleCableMaxSpeedCV = 0.20
	// saddleCablePauseFillerSpdMs / saddleCablePauseFillerMaxFraction —
	// guards against synthetic recording-pause fillers that show speed=0
	// for many samples but live on the stream as 1Hz rows. A real cable
	// has < 5 % zero-speed samples (boarding/disembark only).
	saddleCablePauseFillerSpdMs       = 0.1
	saddleCablePauseFillerMaxFraction = 0.05
	// saddleCableMinAscM / saddleCableMinDescM — both directions must
	// carry real alt mass. A cable hanging in catenary between two
	// stations has both ascent (toward each tower) and descent (between
	// towers); the saddle profile is the physical signature. Pure-
	// monotonic climbs (steady uphill road, walked approach to boarding)
	// have one direction near zero and must be left to the upstream
	// detector — admitting them here opens a wide FP class.
	saddleCableMinAscM  = 5.0
	saddleCableMinDescM = 5.0
	// saddleCableMaxAnchorDistM — radius around an existing detected
	// ride's endpoint that counts as a position anchor for the recovery
	// candidate. 100 m absorbs GPS noise plus station-platform extent
	// (gondola halls span 30–50 m); a stricter 30 m would reject the
	// calibrated saddle-cable case anchors that match within 25–30 m.
	// A looser 200 m would start admitting unrelated stream segments.
	saddleCableMaxAnchorDistM = 100.0
	// saddleCableMaxSinuosity — gondola cables hang in catenaries
	// between towers; the GPS path is a near-straight line from
	// boarding to disembark with sinuosity ≤ 1.10 (cum-dist barely
	// exceeds the chord). A bikepark / cycling connector loop at a
	// lift-top measures 5–10 — the user winds around a small area at
	// cable-class speed. The 1.20 cap admits real cables with GPS
	// jitter headroom and rejects loop-back motion.
	saddleCableMaxSinuosity = 1.20
)

// anchorPos is one (lat, lng) reference point lifted from an input ride's
// start or end. The recovery candidate must sit within
// saddleCableMaxAnchorDistM of an anchor at BOTH endpoints.
type anchorPos struct {
	lat, lng float64
}

// recoverSaddleCables scans rows for fast-flat saddle-cable segments not
// already claimed by `in` and emits them as bidir rides. Returned slice
// preserves chronological order: existing rides + recovered rides merged
// by start time.
func recoverSaddleCables(in []rideWithIdx, rows []StreamPoint, cfg DetectorConfig) []rideWithIdx {
	if len(rows) < int(saddleCableMinDurationS) {
		return in
	}
	// Anchor positions: every input ride's start/end coordinate. The
	// dual-anchor gate downstream rejects when a candidate's endpoints
	// don't sit near any of these — without input rides (cycling
	// activities) anchors is empty and the gate rejects every candidate.
	anchors := make([]anchorPos, 0, 2*len(in))
	for _, r := range in {
		if r.startIdx >= 0 && r.startIdx < len(rows) {
			if pt := rows[r.startIdx]; pt.Lat != nil && pt.Lng != nil {
				anchors = append(anchors, anchorPos{*pt.Lat, *pt.Lng})
			}
		}
		if r.endIdx >= 0 && r.endIdx < len(rows) {
			if pt := rows[r.endIdx]; pt.Lat != nil && pt.Lng != nil {
				anchors = append(anchors, anchorPos{*pt.Lat, *pt.Lng})
			}
		}
	}
	// Coverage mask: a sample is "covered" if it falls inside any input
	// ride window. The recovery pass only emits over uncovered ranges
	// to avoid double-counting.
	covered := make([]bool, len(rows))
	for _, r := range in {
		lo, hi := r.startIdx, r.endIdx
		if lo < 0 {
			lo = 0
		}
		if hi > len(rows)-1 {
			hi = len(rows) - 1
		}
		for k := lo; k <= hi; k++ {
			covered[k] = true
		}
	}

	out := make([]rideWithIdx, 0, len(in)+2)
	out = append(out, in...)

	// Walk uncovered spans; for each span, try to find the longest
	// sub-window that passes every gate. At most one recovery per span.
	i := 0
	for i < len(rows) {
		if covered[i] {
			i++
			continue
		}
		spanStart := i
		for i < len(rows) && !covered[i] {
			i++
		}
		spanEnd := i - 1
		cs, ce, ok := findSaddleCableInSpan(rows, spanStart, spanEnd, anchors)
		if !ok {
			continue
		}
		st := computeStats(rows, cs, ce, cfg)
		out = append(out, rideWithIdx{
			ride: LiftRide{
				StartedAt:  rows[cs].Time,
				EndedAt:    rows[ce].Time,
				DurationS:  int(rows[ce].Time.Sub(rows[cs].Time).Seconds()),
				AscentM:    st.altPosDelta,
				DescentM:   st.altNegDelta,
				DistanceM:  st.cumDist,
				Confidence: "bidir",
			},
			startIdx: cs,
			endIdx:   ce,
		})
	}

	sort.SliceStable(out, func(a, b int) bool {
		return out[a].ride.StartedAt.Before(out[b].ride.StartedAt)
	})
	return out
}

// findSaddleCableInSpan locates the longest contiguous window inside
// [spanStart, spanEnd] that satisfies every saddle-cable gate including
// the dual-position-anchor check. Returns (-, -, false) when no such
// window exists.
//
// Strategy: O(N²) double scan with rolling per-sample stats. For each
// candidate start position, extend `end` one sample at a time and check
// every aggregate gate against the rolling sums. The dual-anchor check
// runs last (after the cheap aggregate gates) using fresh end-position
// only — start-position match is computed once per outer iteration.
// Keep the latest valid `end` for each start (gives the longest
// qualifying window with this anchor); across all starts, keep the
// globally longest.
func findSaddleCableInSpan(rows []StreamPoint, spanStart, spanEnd int, anchors []anchorPos) (int, int, bool) {
	bestStart, bestEnd, bestDur := -1, -1, 0.0
	for start := spanStart; start <= spanEnd; start++ {
		startPt := rows[start]
		if startPt.SpeedMs == nil || startPt.Lat == nil || startPt.Lng == nil {
			continue
		}
		// Cheap up-front: the start position must anchor against an
		// existing ride. If not, no end-extension can rescue it.
		if !anchorMatches(*startPt.Lat, *startPt.Lng, anchors) {
			continue
		}

		var sumSpd, sumSqSpd float64
		count, zeroCount := 0, 0
		posAlt, negAlt := 0.0, 0.0
		prevAlt := math.NaN()
		// Rolling cumulative haversine path length from `start` to the
		// current end. Used by the sinuosity gate (cumDist / straight).
		cumDist := 0.0
		prevLat, prevLng := *startPt.Lat, *startPt.Lng

		localBestEnd := -1
		for end := start; end <= spanEnd; end++ {
			if rows[end].SpeedMs != nil {
				spd := *rows[end].SpeedMs
				sumSpd += spd
				sumSqSpd += spd * spd
				count++
				if spd < saddleCablePauseFillerSpdMs {
					zeroCount++
				}
			}
			if rows[end].AltitudeM != nil {
				a := *rows[end].AltitudeM
				if !math.IsNaN(prevAlt) {
					d := a - prevAlt
					switch {
					case d > 0:
						posAlt += d
					case d < 0:
						negAlt -= d
					}
				}
				prevAlt = a
			}
			if end > start && rows[end].Lat != nil && rows[end].Lng != nil {
				cumDist += Haversine(prevLat, prevLng, *rows[end].Lat, *rows[end].Lng)
				prevLat, prevLng = *rows[end].Lat, *rows[end].Lng
			}
			durS := rows[end].Time.Sub(rows[start].Time).Seconds()
			if durS < saddleCableMinDurationS {
				continue
			}
			if count < 1 {
				continue
			}
			mean := sumSpd / float64(count)
			if mean < saddleCableMinSpeedMs || mean > saddleCableMaxSpeedMs {
				continue
			}
			variance := (sumSqSpd / float64(count)) - mean*mean
			if variance < 0 {
				variance = 0
			}
			cv := math.Sqrt(variance) / mean
			if cv > saddleCableMaxSpeedCV {
				continue
			}
			if float64(zeroCount)/float64(count) > saddleCablePauseFillerMaxFraction {
				continue
			}
			mass := posAlt + negAlt
			if mass < saddleCableMinMassM || mass > saddleCableMaxMassM {
				continue
			}
			if posAlt < saddleCableMinAscM || negAlt < saddleCableMinDescM {
				continue
			}
			endPt := rows[end]
			if endPt.Lat == nil || endPt.Lng == nil {
				continue
			}
			// Sinuosity gate: cum-dist over the path versus straight-
			// line chord. Real cable catenaries are nearly straight
			// (sin ≈ 1.0–1.10); a curved bikepark connector at a
			// lift-top sees sin ≥ 5. Cheap divisor guard.
			straight := Haversine(*startPt.Lat, *startPt.Lng, *endPt.Lat, *endPt.Lng)
			if straight < 1.0 {
				straight = 1.0
			}
			if cumDist/straight > saddleCableMaxSinuosity {
				continue
			}
			// Dual-position-anchor gate (final check). Start was
			// anchored at outer-loop entry; end must also anchor.
			if !anchorMatches(*endPt.Lat, *endPt.Lng, anchors) {
				continue
			}
			localBestEnd = end
		}
		if localBestEnd < 0 {
			continue
		}
		durS := rows[localBestEnd].Time.Sub(rows[start].Time).Seconds()
		if durS > bestDur {
			bestStart, bestEnd, bestDur = start, localBestEnd, durS
		}
	}
	if bestStart < 0 {
		return 0, 0, false
	}
	return bestStart, bestEnd, true
}

// anchorMatches returns true when (lat, lng) sits within
// saddleCableMaxAnchorDistM metres of any anchor.
func anchorMatches(lat, lng float64, anchors []anchorPos) bool {
	for _, a := range anchors {
		if Haversine(lat, lng, a.lat, a.lng) <= saddleCableMaxAnchorDistM {
			return true
		}
	}
	return false
}
