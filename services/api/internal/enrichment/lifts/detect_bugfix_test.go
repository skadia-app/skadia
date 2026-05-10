package lifts

// Bug-pattern fixtures derived from the SyntheticCableTraverse corpus activity
// (336bc170-685f-40f2-8aef-9d0576727e96, alpine_skiing, 2026-03-20).
// Each test reproduces a specific detector failure observed against
// the real stream as a minimal synthetic stream so the regression is
// visible in unit-test scope rather than only in the eval harness.
//
// All four tests start RED and turn GREEN as the matching detector fix
// lands. They share the same coordinate convention as the other
// canonical fixtures: x_m / y_m are local metric coordinates, mapped
// to WGS-84 via projectToWGS84 (lat=47, lng=12 origin).

import (
	"math"
	"testing"
	"time"
)

// streamPoint is a tiny builder for StreamPoint with x/y/alt/speed in
// metric units. Time spacing is 1 Hz, monotonic. Returns a StreamPoint
// using the same lat/lng projection as synthCableTrack.
func streamPoint(t time.Time, xM, yM, altM, speedMs float64) StreamPoint {
	lat, lng := projectToWGS84(xM, yM)
	a, s := altM, speedMs
	return StreamPoint{
		Time:      t,
		Lat:       &lat,
		Lng:       &lng,
		AltitudeM: &a,
		SpeedMs:   &s,
	}
}

// TestBugA_SlowChairliftWithSkiingApproach reproduces Lift E in the
// SyntheticCableTraverse corpus: a slow chairlift (≈2 m/s, ascent rate ≈0.6 m/s)
// preceded by a 10s ski approach (sinuous high-speed in) and a 30 s
// boarding plateau (near-stationary at the lower station). The
// detector latches an initial window mid-cable. Backward-extension
// walks through the boarding plateau and into the ski approach, the
// resulting window's full-window sinuosity exceeds MaxSinuosity and
// validateRide rejects the extension; the ride is left starting half-
// way up the cable, missing ≈40 % of ascent and the entire boarding
// boundary.
//
// Fix expectation (mini-sin reset): when the per-slice sinuosity probe
// in extendCandidateBackward fires, walking back stops at the last
// clean sliceTopIdx instead of letting `start` sit inside the spike.
// The ride then spans boarding-end → cable-end with start near the
// boarding-anchor.
func TestBugA_SlowChairliftWithSkiingApproach(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildSlowChairliftWithSkiingApproach(base)

	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != 1 {
		t.Fatalf("got %d rides, want 1 (slow cable should be a single ride from boarding to top)", len(rides))
	}
	// Ride must START near the boarding anchor (x=300m), not mid-cable.
	// The slice-top alignment lands within ~30 s of boarding-end.
	startOffsetS := rides[0].StartedAt.Sub(base.Add(60 * time.Second)).Seconds()
	if math.Abs(startOffsetS) > 35 {
		t.Errorf("ride start offset from boarding-end = %.0fs, want |offset| ≤ 35s", startOffsetS)
	}
	// And END near the top anchor (cable-end at t=540s).
	endOffsetS := rides[0].EndedAt.Sub(base.Add(540 * time.Second)).Seconds()
	if math.Abs(endOffsetS) > 20 {
		t.Errorf("ride end offset from cable-end = %.0fs, want |offset| ≤ 20s", endOffsetS)
	}
	// A ride that captures boarding→top sees ~300 m of cable ascent.
	// Without the fix the detection lands in the upper third (~180 m).
	if rides[0].AscentM < 260 {
		t.Errorf("ascent = %.1f m, want ≥ 260 m (fix should capture boarding+full cable)", rides[0].AscentM)
	}
}

// buildSlowChairliftWithSkiingApproach assembles a 540 s synthetic
// stream that mirrors the SyntheticCableTraverse Lift-E first ascent shape.
//
//	[0,30) s   ski approach: zig-zag y-axis, x advances 250 m,
//	           altitude drops 80 m. Mean speed ≈ 8 m/s, sinuosity ≫ 1.20
//	           (forces the backward-walk mini-sin probe to fire).
//	[30,60) s  boarding plateau: x almost stationary at 300 m, altitude
//	           flat at 1900 m, speed in walking band (~1 m/s).
//	[60,540) s slow cable: x advances 1200 m linearly, altitude rises
//	           1900 → 2200 m at ~0.625 m/s, speed steady 2.5 m/s.
//
// The cable phase satisfies validateRide on its own (sin≈1, CV≈0,
// |altRate|≈0.625, gradient≈0.25, asc=300m, dur=480s). The detector
// without the fix rejects the extension once it walks back through
// the ski approach because the full window sinuosity rises above
// MaxSinuosity.
func buildSlowChairliftWithSkiingApproach(base time.Time) []StreamPoint {
	pts := make([]StreamPoint, 0, 541)
	for i := 0; i <= 540; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		var x, y, alt, v float64
		switch {
		case i < 30:
			// Ski approach: x linear 0→250, y oscillates ±18 m every 4 s.
			x = (250.0 / 30.0) * float64(i)
			y = 18 * math.Sin(float64(i)/2.0)
			alt = 1980 - (80.0/30.0)*float64(i)
			v = 8.0
		case i < 60:
			// Boarding plateau: park near (300,0), tiny shuffle.
			x = 295 + 0.2*float64(i-30)
			y = 0
			alt = 1900
			v = 1.0
		default:
			// Cable: x linear 300→1500, altitude rises 1900→2200.
			frac := float64(i-60) / 480.0
			x = 300 + 1200*frac
			y = 0
			alt = 1900 + 300*frac
			v = 2.5
		}
		pts = append(pts, streamPoint(ts, x, y, alt, v))
	}
	return pts
}

// TestBugB_GPSFrozenCableTraverse reproduces the Lift-E split observed
// at 12:27:24–12:28:14 in the SyntheticCableTraverse stream: 49 seconds during
// which the GPS recording reports v=0 with the position (lat/lng/alt)
// pinned at the last fix while the cable physically keeps moving the
// rider upward. When the GPS resumes the next sample shows a +32 m
// altitude jump and a +100 m horizontal jump (with a fictitious haver-
// sine speed of ~96 m/s for that single sample). The detector closes
// the first ride at the freeze and starts a fresh ride after the
// resume; mergeMidLiftSplits cannot merge them because dist(prev.end ↔
// cur.start) ≈ 100 m violates both the cable-hold continuous tier
// (≤15 m) and the smart-recording tier (gapHasSamples=true).
//
// Fix expectation (GPS-frozen cable-traverse merge tier): a fourth
// merge tier admits gap≤120 s when every in-gap sample is anchored to
// prev.end (drift≤5 m) AND every in-gap sample reports v≈0 AND the
// end↔start jump is consistent with prev_speed × gap and prev_altRate
// × gap (each within 50 % tolerance). Both halves merge into one ride.
func TestBugB_GPSFrozenCableTraverse(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildGPSFrozenCableTraverse(base)

	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != 1 {
		t.Fatalf("got %d rides, want 1 (GPS-frozen mid-cable must merge with the cable continuation)", len(rides))
	}
	// Start within 20 s of cable-start (t=0).
	startOffsetS := rides[0].StartedAt.Sub(base).Seconds()
	if startOffsetS > 30 {
		t.Errorf("ride start offset = %.0fs, want ≤ 30s", startOffsetS)
	}
	// End within 20 s of cable-end (t=300s, including frozen 49s).
	endOffsetS := rides[0].EndedAt.Sub(base.Add(300 * time.Second)).Seconds()
	if math.Abs(endOffsetS) > 20 {
		t.Errorf("ride end offset = %.0fs, want |offset| ≤ 20s", endOffsetS)
	}
	// Ascent is the sum of both halves (~150 m).
	if rides[0].AscentM < 130 {
		t.Errorf("ascent = %.1f m, want ≥ 130 m (full traverse)", rides[0].AscentM)
	}
}

// buildGPSFrozenCableTraverse: 0–100s cable up, 100–149s GPS frozen
// (samples pinned at last fix, v=0), 150–300s cable up resumes. The
// resume-sample at t=150 lands ≈100 m further along x and ≈30 m higher
// — a position consistent with prev_speed × gap and prev_altRate × gap.
func buildGPSFrozenCableTraverse(base time.Time) []StreamPoint {
	pts := make([]StreamPoint, 0, 301)
	const cableSpeed = 2.0 // m/s
	const altRate = 0.6    // m/s
	for i := 0; i <= 300; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		var x, y, alt, v float64
		y = 0
		switch {
		case i < 100:
			// Pre-freeze cable.
			x = cableSpeed * float64(i)
			alt = 1900 + altRate*float64(i)
			v = cableSpeed
		case i < 150:
			// GPS frozen: pin to the last pre-freeze fix (i=99).
			x = cableSpeed * 99.0
			alt = 1900 + altRate*99.0
			v = 0
		default:
			// Post-freeze cable: position consistent with the lift having
			// kept moving during the freeze (no jump elsewhere).
			x = cableSpeed * float64(i)
			alt = 1900 + altRate*float64(i)
			v = cableSpeed
		}
		pts = append(pts, streamPoint(ts, x, y, alt, v))
	}
	return pts
}

// TestBugA3_RealStreamShufflePatternBlocksBoardingAbsorption mirrors
// the *full* shape of SyntheticCableTraverse Lift E Auffahrt 2 — every feature
// that a synthetic test could plausibly miss is included here, because
// the simpler TestBugA2 stream alone is insufficient to drive the
// detector through the exact fall-through that defeats the boarding-
// absorption fix:
//
//   1. Pre-boarding ski approach (30 s at 7-13 m/s) — alt rises
//      from 1898 m at the lift station back up to 1980 m the rider
//      came from. Walking back into this region from inside the lift
//      candidate trips extendCandidateBackward's alt-reversal break
//      (peakDropThreshM = 12 m above initialAlt) before alt-stall has
//      time to fire.
//   2. 35 s boarding-shuffle plateau — velocity jitters between 0.2
//      and 1.2 m/s (alternating stop/non-stop samples). The forward
//      stop-overflow gate (backwardStopToleranceS = 30 s) cannot
//      accumulate 30 s of contiguous stop because the > 0.7 m/s
//      samples reset stopAccumS each time. The plateau is exactly at
//      altAdvanceStallBackS = 45 s threshold's edge, so stall does
//      not have a clean window to fire either.
//   3. Slow-cable Anfang (115 s, alt-rate 0.30, mean speed 2.0 m/s) —
//      satisfies the trimRegionMeanSpeed > 1.5 m/s condition so the
//      A2 fix's clamp relaxation activates and the backward walk DOES
//      run (where TestBugA2 ends). The walk however now reaches all
//      the way through the boarding plateau and into the ski approach
//      before alt-reversal fires, and validateRide rejects the merged
//      window for sin > MaxSinuosity (the ski-zigzag dominates).
//
// Bug result: the BACK-EXT computeStats delivers a window with
// sinuosity ≈ 1.5 (ski-zigzag absorbed), validateRide returns false,
// and DetectHeuristic falls back to ride.StartedAt = post-trim start
// (≈ 180 s, the slow-cable / stable-cable boundary). Ride start drift
// from the boarding-plateau end (t = 65 s) is ≈ 115 s ≈ 230 m of
// horizontal distance — well outside the boundary tolerance band.
//
// Fix expectation: when alt-reversal fires walking backward, pin
// start to lastAdvanceIdx (mirror of alt-stall's existing pinning
// behaviour). The cable's last advancing sample is the boarding-
// plateau boundary; pinning start there preserves the boarding
// absorption while still rejecting the ski-approach samples beyond.
// validateRide then sees sin ≈ 1.0 and admits the back-extension.
func TestBugA3_RealStreamShufflePatternBlocksBoardingAbsorption(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildLiftWithPreSkiAndShufflePlateau(base)

	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != 1 {
		t.Fatalf("got %d rides, want 1", len(rides))
	}

	// Boarding-plateau end at t = 65 s. The fix should land start
	// within ~30 s (one Phase-3 advance step at 0.30 m/s clears 3 m
	// every 10 s).
	expectedStart := base.Add(65 * time.Second)
	startOffsetS := rides[0].StartedAt.Sub(expectedStart).Seconds()
	if math.Abs(startOffsetS) > 30 {
		t.Errorf("ride start offset from boarding-end = %+.0f s, want |offset| ≤ 30 s — alt-reversal fired before alt-stall could pin start to lastAdvanceIdx",
			startOffsetS)
	}

	// AscentM with the boarding boundary captured: alt[65]=1898 →
	// alt[540]=2189 ≈ 290 m. Without the fix the post-trim start at
	// t=180 caps ascent at ≈ 256 m. The 270 m threshold separates
	// buggy (256) from fixed (290) by the slow-anfang's 35 m
	// contribution.
	if rides[0].AscentM < 270 {
		t.Errorf("ascent = %.1f m, want ≥ 270 m — slow-anfang vertical lost because backward walk validate failed on sin from ski approach",
			rides[0].AscentM)
	}
}

func buildLiftWithPreSkiAndShufflePlateau(base time.Time) []StreamPoint {
	pts := make([]StreamPoint, 0, 541)
	for i := 0; i <= 540; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		var x, y, alt, v float64
		switch {
		case i < 30:
			// Ski approach: zig-zag y, x advances 250 m, altitude
			// drops 2050 → 1898 m at 5.07 m/s (steep ski-down to the
			// lift's valley station). The starting altitude has to be
			// high enough that walking backward from inside the cable
			// trips alt-reversal (initialAlt + peakDropThreshM = 12 m)
			// at *the same iteration* alt-stall would otherwise fire,
			// so the alt-reversal handler's start-pinning behaviour is
			// what we actually exercise. Mean speed ≈ 8 m/s, sinuosity
			// ≫ 1.20 (drives the eventual back-extension's full-window
			// sin over MaxSinuosity if absorbed).
			x = (250.0 / 30.0) * float64(i)
			y = 18 * math.Sin(float64(i)/2.0)
			alt = 2050 - (152.0/30.0)*float64(i)
			v = 8.0
		case i < 65:
			// Boarding shuffle: 35 s plateau at alt 1898 m. Speed
			// jitters with period 5 s between 0.2 and 1.2 m/s — every
			// other sample sits above MinSpeedMs (0.7) so the backward
			// stop branch's stopAccumS resets repeatedly and never
			// trips backwardStopToleranceS = 30 s.
			x = 295 + 0.2*float64(i-30)
			y = 0
			alt = 1898
			t := float64(i - 30)
			v = 0.7 + 0.5*math.Sin(2.0*math.Pi*t/5.0)
		case i < 180:
			// Slow-cable Anfang: stable speed 2.0 m/s, alt-rate
			// 0.30 m/s. trimRegionMeanSpeed sees mean = 2.0 → A2 fix's
			// clamp relaxation enables backward walk past preTrimStart.
			t := float64(i - 65)
			x = 302 + 2.0*t
			y = 0
			alt = 1898 + 0.30*t
			v = 2.0
		default:
			// Stable cable: speed 2.5 m/s, alt-rate 0.71 m/s. First
			// sample where leadRate ≥ leadAbsCapMs (0.40) — trim halts
			// here and ride.StartedAt sits at this boundary in the
			// buggy baseline.
			t := float64(i - 180)
			x = 302 + 230 + 2.5*t
			y = 0
			alt = 1933 + 0.71*t
			v = 2.5
		}
		pts = append(pts, streamPoint(ts, x, y, alt, v))
	}
	return pts
}

// TestBugC2_PureDescentCable reproduces seg1 of the SyntheticCableTraverse return
// trip (Mid-Station → valley): a cable that only descends, ≈515 m
// altitude loss over 5 minutes at 4–5 m/s with sinuosity ≈1.02. The
// classify "high" branch requires altRate ≥ 0.05 (signed, positive)
// so a pure descent never enters that branch. The bidir branch is
// reachable but downstream gates currently reject because the ride
// has no positive ascent component.
//
// Fix expectation (pure-descent classify): classify accepts a pure
// downhill cable, either by relaxing "high" to |altRate| ≥ 0.05 or by
// admitting bidir when |altRate| ≥ 0.4 m/s and sin ≤ BidirSinuosityMax.
// validateRide must accept descent-only with absAlt ≥ 40 m floor on
// max(altPosDelta, altNegDelta).
func TestBugC2_PureDescentCable(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildPureDescentCable(base)

	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != 1 {
		t.Fatalf("got %d rides, want 1 (pure descent cable must register as a single lift ride)", len(rides))
	}
	if rides[0].DescentM < 450 {
		t.Errorf("descent = %.1f m, want ≥ 450 m (full cable down)", rides[0].DescentM)
	}
	if rides[0].AscentM > 30 {
		t.Errorf("ascent = %.1f m, want ≤ 30 m (pure descent)", rides[0].AscentM)
	}
}

// buildPureDescentCable: 0–300 s, alt 2415 → 1900 (-515 m), x 0 → 1500
// (linear, sinuosity ≈ 1.0), v steady 5 m/s. No ski approach, no
// boarding plateau — clean cable descent only.
func buildPureDescentCable(base time.Time) []StreamPoint {
	pts := make([]StreamPoint, 0, 301)
	for i := 0; i <= 300; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		x := 5.0 * float64(i)
		y := 0.0
		alt := 2415 - (515.0/300.0)*float64(i)
		v := 5.0
		pts = append(pts, streamPoint(ts, x, y, alt, v))
	}
	return pts
}

// TestBugC1_SaddleCrossingBidir reproduces seg3 of the SyntheticCableTraverse
// return trip (top station → mid-station 2 across a saddle): a cable
// whose net altitude change is ≈0 m over 200 s but which goes up
// 30 m, peaks at 2127 m, then down 25 m. Sum of |Δalt| ≈ 55 m, the
// straight-line shape stays sinuous ≈1.02. classify in the initial
// 30 s window currently has trouble because the boarding-phase
// volatility pumps speedCV above MaxSpeedCV; downstream the saddle
// shape (small net ascent, modest |Δalt|) doesn't pass the high-tier
// gate, and the bidir branch needs sufficient |Σ Δalt| to clear the
// asc/desc floor. The detector therefore sees no ride at all.
//
// Fix expectation: a saddle-shaped bidir candidate (|Σ Δalt| ≥
// BidirAltAbsM AND sin ≤ BidirSinuosityMax AND duration ≥
// BidirMinDurationS) is admitted by classify regardless of the
// instantaneous altRate sign of the initial window, and validateRide
// accepts bidir when max(altPosDelta, altNegDelta) ≥ ~25 m AND |Σ
// Δalt| ≥ BidirAltAbsM.
func TestBugC1_SaddleCrossingBidir(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildSaddleCrossingBidir(base)

	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != 1 {
		t.Fatalf("got %d rides, want 1 (saddle bidir cable must register as one ride)", len(rides))
	}
	if rides[0].Confidence != "bidir" {
		t.Errorf("confidence = %q, want bidir for saddle crossing", rides[0].Confidence)
	}
	// Sum of absolute deltas should be ≥ 50 m even though net is ~0.
	totalAbs := rides[0].AscentM + rides[0].DescentM
	if totalAbs < 45 {
		t.Errorf("|asc|+|desc| = %.1f m, want ≥ 45 m", totalAbs)
	}
}

// TestBugA2_TrimClampBlocksBackwardWalk targets the exact failure mode
// observed at SyntheticCableTraverse Lift E Auffahrt 2 (12:21:55 → 12:33:30):
//
//   1. The slow-cable-anfang has a substantially smaller altitude rate
//      than the rest of the ride. trimLeadingLowGradient trims those
//      leading-low-gradient samples forward (because leadRate <
//      leadRelativeRatio × restRate AND leadRate < leadAbsCapMs).
//   2. After trim moved start forward, DetectHeuristic clamps the
//      backward-walk lowerBound to the *trimmed* start:
//
//          if start > preTrimStart && lowerBound < start {
//              lowerBound = start
//          }
//
//      This pins extendCandidateBackward's lowerBoundIdx to the post-
//      trim position. Its entry guard `if oldStart <= lowerBoundIdx
//      { return oldStart }` then short-circuits — the function returns
//      without ever walking, even though the actual cable boarding-
//      plateau end (the *true* lift start) sits well before preTrimStart.
//
//   3. With no walk, the ride's StartedAt is fixed at the trimmed
//      start, deep inside the cable phase. The boarding-plateau end
//      is missed and AscentM is short by the slow-anfang's vertical
//      contribution (~50 m in the calibrated case).
//
// Synthetic stream shape (1 Hz, 540 s):
//
//   [0, 60)   s   boarding plateau — alt 1900 m flat, walking-pace
//                 speed 0.6 m/s. 60 s span guarantees the backward
//                 stop-overflow gate (backwardStopToleranceS = 30 s)
//                 has room to fire inside the plateau, pinning the
//                 walk's terminal `start` at the plateau's end.
//   [60, 160) s   slow cable Anfang — alt-rate 0.30 m/s, speed 2.0 m/s.
//                 leadRate (0.30) < leadAbsCapMs (0.40) and leadRate
//                 (0.30) < leadRelativeRatio (0.5) × restRate (~0.65)
//                 → trim fires repeatedly through this whole region.
//   [160, 540] s  stable cable — alt-rate 0.71 m/s, speed 2.5 m/s.
//                 First sample where leadRate ≥ leadAbsCapMs, so trim
//                 stops here; this is the post-trim ride-start under
//                 the current bug.
//
// Without the fix the detector reports ride.StartedAt ≈ 160 s; with
// the fix backward-walk crosses preTrimStart and the boarding stop-
// overflow pins ride.StartedAt ≈ 60 s. The ascent gap between the two
// outcomes is ~50 m of slow-anfang vertical the bug is currently
// discarding.
func TestBugA2_TrimClampBlocksBackwardWalk(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildLiftWithTrimmedSlowAnfang(base)

	rides := DetectHeuristic(pts, DefaultDetectorConfig("ski"))
	if len(rides) != 1 {
		t.Fatalf("got %d rides, want 1", len(rides))
	}

	// Boarding ends at t=60 s; the stop-overflow gate inside the
	// plateau pins start at the plateau boundary within 25 s on a 1 Hz
	// stream (backwardStopToleranceS = 30 s, plus one decel sample).
	expectedStart := base.Add(60 * time.Second)
	startOffsetS := rides[0].StartedAt.Sub(expectedStart).Seconds()
	if math.Abs(startOffsetS) > 25 {
		t.Errorf("ride start offset from boarding-end = %+.0f s, want |offset| ≤ 25 s — clamp likely blocked backward-walk past preTrimStart",
			startOffsetS)
	}

	// AscentM with the full ride captured: 1900 → 2200 m ≈ 300 m.
	// Without fix the post-trim start at t=160 caps ascent at
	// ≈ 270 m (alt[160]=1930 → alt[540]=2200 = 270). The 280 m
	// threshold therefore separates buggy from fixed behaviour by
	// the slow-anfang's 30 m contribution alone.
	if rides[0].AscentM < 280 {
		t.Errorf("ascent = %.1f m, want ≥ 280 m — slow-anfang vertical lost to the trim+clamp",
			rides[0].AscentM)
	}

	// End boundary should land near t=540 s (top of cable).
	expectedEnd := base.Add(540 * time.Second)
	endOffsetS := rides[0].EndedAt.Sub(expectedEnd).Seconds()
	if math.Abs(endOffsetS) > 20 {
		t.Errorf("ride end offset from cable-end = %+.0f s, want |offset| ≤ 20 s",
			endOffsetS)
	}
}

func buildLiftWithTrimmedSlowAnfang(base time.Time) []StreamPoint {
	pts := make([]StreamPoint, 0, 541)
	for i := 0; i <= 540; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		var x, y, alt, v float64
		y = 0
		switch {
		case i < 60:
			// Boarding plateau: walking-pace speed (well below
			// MinSpeedMs=0.7) so the backward-walk stop branch can
			// accumulate ≥30 s of stop and trip backwardStopToleranceS,
			// pinning start at the plateau boundary.
			x = 0.2 * float64(i)
			alt = 1900
			v = 0.6
		case i < 160:
			// Slow cable Anfang: stable speed 2.0 m/s, alt-rate 0.30 m/s.
			// Initial-window classifies "high" here (gradient
			// 0.30/2.0 = 0.15 > MinGradient 0.10, CV ≈ 0). Trim then
			// peels these samples forward because their alt-rate is
			// less than half the rest-of-ride alt-rate (0.71/2 = 0.355,
			// 0.30 < 0.355).
			t := float64(i - 60)
			x = 12 + 2.0*t
			alt = 1900 + 0.30*t
			v = 2.0
		default:
			// Stable cable: alt-rate 0.71 m/s, speed 2.5 m/s. First
			// sample where leadRate ≥ leadAbsCapMs (0.40), so trim
			// halts at this boundary.
			t := float64(i - 160)
			x = 12 + 200 + 2.5*t
			alt = 1930 + 0.71*t
			v = 2.5
		}
		pts = append(pts, streamPoint(ts, x, y, alt, v))
	}
	return pts
}

// buildSaddleCrossingBidir mirrors the SyntheticCableTraverse return-trip seg3 cable
// (top-station → mid-station-2, ≈ 11:29:23 → 11:32:35 in the corpus
// activity). The cable dips ≈10 m, peaks ≈32 m above start, then descends
// ≈24 m to land 5 m below start — net ≈‑5 m, |Σ Δalt| ≈ 66 m. Sin ≈ 1.02.
//
// Stream layout (1 Hz, 200 s):
//
//	[0,30)   s   pre-cable shuffle/boarding ramp — alt 2108 m flat,
//	             speed 0.4–4.5 m/s ramp into cable. Pumps the initial
//	             classify-window's CV.
//	[30,55)  s   cable down 1 — alt 2108 → 2098 (≈‑10 m, rate ‑0.4 m/s),
//	             speed 4.8 m/s steady.
//	[55,130) s   cable up — alt 2098 → 2130 (≈+32 m, rate 0.43 m/s),
//	             speed 4.8 m/s steady.
//	[130,180) s  cable down 2 — alt 2130 → 2103 (≈‑27 m, rate ‑0.54 m/s),
//	             speed 4.8 m/s steady.
//	[180,200) s  mid-station idle — alt 2103 m flat, speed 0.5 m/s.
//
// altPos+altNeg ≈ 32 + 10 + 27 = 69 m; sin = 1.0; both > the bidir
// floors. The detector still misses this in practice because the
// up-phase windows classify "high" and break on peakDrop before
// validateRide accepts; the saddle is never seen as one bidir ride.
func buildSaddleCrossingBidir(base time.Time) []StreamPoint {
	pts := make([]StreamPoint, 0, 201)
	for i := 0; i <= 200; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		var x, alt, v float64
		switch {
		case i < 30:
			// Pre-cable shuffle ramp.
			x = 0.5 * float64(i)
			alt = 2108.0
			v = 0.4 + (4.4/29.0)*float64(i)
		case i < 55:
			// Cable down 1: 2108 → 2098.
			frac := float64(i-30) / 25.0
			x = 15.0 + 4.8*float64(i-30)
			alt = 2108.0 - 10.0*frac
			v = 4.8
		case i < 130:
			// Cable up to peak: 2098 → 2130.
			frac := float64(i-55) / 75.0
			x = 15.0 + 4.8*25.0 + 4.8*float64(i-55)
			alt = 2098.0 + 32.0*frac
			v = 4.8
		case i < 180:
			// Cable down 2: 2130 → 2103.
			frac := float64(i-130) / 50.0
			x = 15.0 + 4.8*25.0 + 4.8*75.0 + 4.8*float64(i-130)
			alt = 2130.0 - 27.0*frac
			v = 4.8
		default:
			// Mid-station idle: alt-flat at 2103.
			x = 15.0 + 4.8*25.0 + 4.8*75.0 + 4.8*50.0 + 0.5*float64(i-180)
			alt = 2103.0
			v = 0.5
		}
		pts = append(pts, streamPoint(ts, x, 0, alt, v))
	}
	return pts
}

// TestBugD_SplitAtMidStations_DetectsSignatureAndSplits unit-tests the
// post-detection split pass in isolation, bypassing the upstream
// detector. Given a synthetic stream that mirrors the SyntheticCableTraverse ride-10
// shape (seg2 descent → mid-station-2 plateau → seg3 saddle) and a
// pre-constructed bidir parent ride spanning the entire stream, the
// pass must emit two sub-rides split at the mid-station plateau.
//
// Mid-station signature in the real stream at 09:04:06–09:04:36:
//   - sin spike (1.42 then 1.06) — cable winds around the station
//   - speed dip (≤1.0 m/s for ≥30 s) — cabin briefly halts at platform
//   - alt plateau (|Δalt| < 5 m for ≥30 s) — station floor is flat
//
// Tested in isolation so a synth that does not pass the upstream
// stall-on-descent gate can still drive the splitter — the integration
// path through DetectHeuristic is exercised by the corpus eval (see
// scripts/lift_eval_diff.sh and the b2b_seg3 count assertion in
// the SyntheticCableTraverse GT manifest).
func TestBugD_SplitAtMidStations_DetectsSignatureAndSplits(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildMultiStationGondolaTraversal(base)

	// Pre-constructed bidir parent ride spanning the entire synth stream
	// — simulates what the upstream detector emits for ride 10 in the
	// real corpus (a single bidir ride covering seg2 + seg3 merged).
	parent := rideWithIdx{
		ride: LiftRide{
			StartedAt:  pts[0].Time,
			EndedAt:    pts[len(pts)-1].Time,
			DurationS:  int(pts[len(pts)-1].Time.Sub(pts[0].Time).Seconds()),
			AscentM:    34, // seg3 saddle ascent
			DescentM:   342,
			DistanceM:  1400,
			Confidence: "bidir",
		},
		startIdx: 0,
		endIdx:   len(pts) - 1,
	}

	out := splitAtMidStations([]rideWithIdx{parent}, pts, DefaultDetectorConfig("ski"))
	if len(out) != 2 {
		t.Fatalf("got %d output rides, want 2 (seg2 + seg3 should split at mid-station signature)", len(out))
	}

	// Sub-ride 1 = seg2 descent, ends near the mid-station plateau.
	end1 := out[0].ride.EndedAt.Sub(base).Seconds()
	if end1 < 220 || end1 > 320 {
		t.Errorf("first ride end = %.0f s, want 220-320 s (near mid-station plateau)", end1)
	}
	if out[0].ride.DescentM < 250 {
		t.Errorf("first ride descent = %.1f m, want ≥ 250 m (seg2 mid1→mid2 ≈ 310 m)", out[0].ride.DescentM)
	}

	// Sub-ride 2 = seg3 saddle, starts at the same mid-station boundary.
	start2 := out[1].ride.StartedAt.Sub(base).Seconds()
	if start2 < 220 || start2 > 300 {
		t.Errorf("second ride start = %.0f s, want 220-300 s (resumes from mid-station)", start2)
	}
	if out[1].ride.AscentM < 15 || out[1].ride.DescentM < 15 {
		t.Errorf("second ride saddle: asc=%.1f, desc=%.1f, want both ≥ 15 m (seg3 saddle)",
			out[1].ride.AscentM, out[1].ride.DescentM)
	}

	// The split point must lie strictly between the two rides.
	if out[1].ride.StartedAt.Before(out[0].ride.EndedAt) {
		t.Errorf("rides overlap: out[0].end=%v, out[1].start=%v",
			out[0].ride.EndedAt, out[1].ride.StartedAt)
	}
}

// TestBugD_SplitAtMidStations_NoSignaturePassThrough guards the no-op
// path: a candidate that has no mid-station signature inside its window
// must come out unchanged. Uses the canonical BugC1 saddle stream — a
// single legitimate saddle-bidir cable that should NOT be split.
func TestBugD_SplitAtMidStations_NoSignaturePassThrough(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildSaddleCrossingBidir(base)

	parent := rideWithIdx{
		ride: LiftRide{
			StartedAt:  pts[0].Time,
			EndedAt:    pts[len(pts)-1].Time,
			DurationS:  int(pts[len(pts)-1].Time.Sub(pts[0].Time).Seconds()),
			AscentM:    32,
			DescentM:   37,
			DistanceM:  900,
			Confidence: "bidir",
		},
		startIdx: 0,
		endIdx:   len(pts) - 1,
	}

	out := splitAtMidStations([]rideWithIdx{parent}, pts, DefaultDetectorConfig("ski"))
	if len(out) != 1 {
		t.Errorf("got %d output rides, want 1 (legitimate saddle-bidir, no mid-station)", len(out))
	}
}

// TestBugD_SplitAtMidStations_SkipsNonBidir guards against splitting
// regular (high/medium) rides — those have monotonic profiles and never
// look like multi-station traversals. Pin the contract so a future
// refactor doesn't accidentally widen the scope.
func TestBugD_SplitAtMidStations_SkipsNonBidir(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	pts := buildMultiStationGondolaTraversal(base)

	parent := rideWithIdx{
		ride: LiftRide{
			StartedAt:  pts[0].Time,
			EndedAt:    pts[len(pts)-1].Time,
			Confidence: "high",
		},
		startIdx: 0,
		endIdx:   len(pts) - 1,
	}

	out := splitAtMidStations([]rideWithIdx{parent}, pts, DefaultDetectorConfig("ski"))
	if len(out) != 1 {
		t.Errorf("got %d output rides, want 1 (non-bidir rides must pass through unchanged)", len(out))
	}
}

// buildMultiStationGondolaTraversal mirrors the SyntheticCableTraverse ride-10 shape
// (08:58:53–09:08:31, 578 s, asc=40, desc=346, conf=bidir):
//
//	[0,240)   s seg2 descent — alt 2415 → 2105, x advances 0 → 600 m,
//	            speed 4.0 m/s steady, sin ≈ 1.0 (straight cable).
//	[240,270) s mid-station-2 plateau — x near 600 m, alt 2103 m flat,
//	            speed 0.5 m/s, y zigzags ±5 m for one 30 s block to
//	            drive sinuosity above 1.4 (cable winds around station).
//	[270,510) s seg3 saddle — x advances 600 → 1400 m, alt rises to
//	            2126, dips to 2096, settles at 2107 (saddle profile),
//	            speed 4.0 m/s steady, sin ≈ 1.0.
//
// Plateau is kept at 30 s rather than 60 s because the existing stop-
// resume bridge in the forward extender accumulates ≥60 s before
// emitting BREAK bidir-sin; with 30 s the candidate keeps growing past
// the station and the ride lands as one bidir spanning [0,510]. That is
// the pre-fix RED state this test pins.
func buildMultiStationGondolaTraversal(base time.Time) []StreamPoint {
	pts := make([]StreamPoint, 0, 511)
	for i := 0; i <= 510; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		var x, y, alt, v float64
		switch {
		case i < 240:
			// seg2 descent, linear.
			x = float64(i) * 600.0 / 240.0
			y = 0
			alt = 2415.0 - float64(i)*310.0/240.0
			v = 4.0
		case i < 271:
			// Mid-station plateau (31 samples = 30 s span) with
			// y-zigzag for sin spike.
			x = 600.0
			zig := i - 240
			switch zig % 4 {
			case 0:
				y = 0
			case 1:
				y = 5
			case 2:
				y = 0
			case 3:
				y = -5
			}
			alt = 2103.0
			v = 0.5
		case i < 331:
			// seg3 ascent leg of saddle: 2103 → 2126 over 60 s.
			ti := float64(i - 271)
			x = 600.0 + ti*200.0/60.0
			y = 0
			alt = 2103.0 + ti*23.0/60.0
			v = 4.0
		case i < 451:
			// seg3 descent leg: 2126 → 2096 over 120 s.
			ti := float64(i - 331)
			x = 800.0 + ti*400.0/120.0
			y = 0
			alt = 2126.0 - ti*30.0/120.0
			v = 4.0
		default:
			// seg3 final climb: 2096 → 2107 over 60 s.
			ti := float64(i - 451)
			x = 1200.0 + ti*200.0/60.0
			y = 0
			alt = 2096.0 + ti*11.0/60.0
			v = 4.0
		}
		pts = append(pts, streamPoint(ts, x, y, alt, v))
	}
	return pts
}
