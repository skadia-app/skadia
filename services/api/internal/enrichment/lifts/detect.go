package lifts

import (
	"fmt"
	"math"
	"os"
	"sort"
	"time"
)

// debugTrace dumps every break/branch decision when LIFT_DEBUG_TRACE is set.
var debugTrace = os.Getenv("LIFT_DEBUG_TRACE") != ""

func dprintf(format string, args ...any) {
	if debugTrace {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}


// DetectHeuristic is the Method-B fallback that flags lift-like ride windows
// in the activity stream without consulting OSM data. It uses three confidence
// classes:
//
//   - "high"   — speed-stable + gradient-strict unidirectional ride
//   - "medium" — no usable speed but altitude-rate strong unidirectional ride
//   - "bidir"  — speed-stable, low-sinuosity ride whose net altitude change is
//                small but cumulative |delta_alt| is large (bidirectional aerial tramway-class)
//
// The detector mirrors services/ml/enrichment/lifts.py with edge-case fixes
// for: GPS-delta-speed fallback when the speed sensor reports zero on a
// moving lift; smart-recording gaps; bidirectional rides; intra-lift stops
// with altitude-direction-reversal check on resume; mid-recording starts.
type rideWithIdx struct {
	ride             LiftRide
	startIdx, endIdx int
}

func DetectHeuristic(rows []StreamPoint, cfg DetectorConfig) []LiftRide {
	if len(rows) < cfg.MinSamples {
		return nil
	}
	var raw []rideWithIdx
	i := 0
	prevFinalEnd := -1 // index after which a backward extension may walk; stays -1 until first ride
	for i < len(rows) {
		start, end, ok := buildInitialWindow(rows, i, cfg)
		if !ok {
			i++
			continue
		}
		ws := computeStats(rows, start, end, cfg)
		kind := classify(ws, cfg)
		if kind == "" {
			i++
			continue
		}
		finalEnd := extendCandidate(rows, start, end, kind, ws, cfg)
		final := computeStats(rows, start, finalEnd, cfg)
		if !validateRide(final, kind, cfg) {
			i++
			continue
		}
		if cfg.BoundarySpeedDropEnabled && !boundarySpeedDropOK(rows, start, finalEnd, cfg) {
			i++
			continue
		}
		// Backward-extension: buildInitialWindow only finds a stable lift
		// window once the ride is mid-flight (boarding-phase speed-CV spikes
		// or sinuosity folds at the lower station push classify out of band
		// for the first 30–120 s). Walk start backward sample-by-sample with
		// the same stop set extendCandidate uses going forward, then keep the
		// extension iff computeStats+validateRide still pass — that guards
		// against pulling in pre-boarding bike-track samples.
		lowerBound := prevFinalEnd + 1
		if lowerBound < 0 {
			lowerBound = 0
		}
		if newStart := extendCandidateBackward(rows, start, finalEnd, kind, final, cfg, lowerBound); newStart < start {
			extFinal := computeStats(rows, newStart, finalEnd, cfg)
			// Relaxed CV cap for backward-extension: the boarding phase (queue
			// shuffle 0.1–1.5 m/s ramping into lift band 3–8 m/s over the
			// first 60–180 s) legitimately raises window CV from <0.04 to
			// 0.05–0.08, even after SpeedCVTrimSeconds removes the leading
			// 30 s. Sinuosity (≤MaxSinuosity 1.08) and mean speed (in lift
			// band) remain the hard discriminators against bike-trail
			// false positives — those have sinuosity ≥1.20.
			//
			// Two-tier cap: pristine geometry (sinuosity ≤ 1.05) confirms a
			// true lift cable so authoritatively that we tolerate up to CV=0.15
			// — alpine bikepark ride[02] mountain station case 1 (sin 1.009, CV 0.119) and ride[04]
			// (sin 1.018, CV 0.118) are exactly this shape: aerial tramways /
			// gondolas where the cabin briefly accelerates from boarding speed
			// to lift speed. A bike trail cannot reproduce sin ≤ 1.05 over
			// 200+ samples, so the CV relaxation is safe.
			relaxed := cfg
			if extFinal.sinuosity <= 1.05 {
				relaxed.MaxSpeedCV = 0.15
			} else {
				relaxed.MaxSpeedCV = 0.10
			}
			ok := validateRide(extFinal, kind, relaxed)
			dprintf("BACK-EXT old=%d new=%d t=%s..%s dur=%.0f mean=%.2f cv=%.3f sin=%.3f rate=%.2f -> validate=%v\n",
				start, newStart,
				rows[newStart].Time.Format("15:04:05"), rows[finalEnd].Time.Format("15:04:05"),
				extFinal.durationS, extFinal.meanSpeed, extFinal.speedCV, extFinal.sinuosity, extFinal.altRateMs, ok)
			if ok {
				start = newStart
				final = extFinal
			}
		}
		raw = append(raw, rideWithIdx{
			ride: LiftRide{
				StartedAt:  rows[start].Time,
				EndedAt:    rows[finalEnd].Time,
				DurationS:  int(rows[finalEnd].Time.Sub(rows[start].Time).Seconds()),
				AscentM:    final.altPosDelta,
				DescentM:   final.altNegDelta,
				DistanceM:  final.cumDist,
				Confidence: kind,
			},
			startIdx: start,
			endIdx:   finalEnd,
		})
		prevFinalEnd = finalEnd
		i = finalEnd + 1
	}
	// Mid-lift-split merge pass: a pylon stop, mid-station glitch, or a
	// brief recording pause splits one physical lift ride into two adjacent
	// candidates. Conservative criteria (per "FP > gap-bridging"):
	//   - gap ≤ 20 s wall-clock between r1.End and r2.Start
	//   - geographic distance r1.End ↔ r2.Start ≤ 25 m (one cabin length)
	//   - same direction (both ascend or both descend)
	//   - neither candidate is bidir (bidir crosses both directions; chaining
	//     two ascends is never the right merge)
	//   - merged window still validateRide()s under the original kind
	merged := mergeMidLiftSplits(raw, rows, cfg)
	out := make([]LiftRide, len(merged))
	for k, r := range merged {
		out[k] = r.ride
	}
	return out
}

func mergeMidLiftSplits(in []rideWithIdx, rows []StreamPoint, cfg DetectorConfig) []rideWithIdx {
	if len(in) < 2 {
		return in
	}
	const maxGapS = 20.0
	const maxPosDistM = 25.0
	out := make([]rideWithIdx, 0, len(in))
	out = append(out, in[0])
	for i := 1; i < len(in); i++ {
		cur := in[i]
		prev := out[len(out)-1]
		if prev.ride.Confidence == "bidir" || cur.ride.Confidence == "bidir" {
			out = append(out, cur)
			continue
		}
		gap := cur.ride.StartedAt.Sub(prev.ride.EndedAt).Seconds()
		if gap > maxGapS || gap < 0 {
			out = append(out, cur)
			continue
		}
		pe, cs := rows[prev.endIdx], rows[cur.startIdx]
		if pe.Lat == nil || pe.Lng == nil || cs.Lat == nil || cs.Lng == nil {
			out = append(out, cur)
			continue
		}
		dist := Haversine(*pe.Lat, *pe.Lng, *cs.Lat, *cs.Lng)
		if dist > maxPosDistM {
			out = append(out, cur)
			continue
		}
		prevAsc := prev.ride.AscentM > prev.ride.DescentM
		curAsc := cur.ride.AscentM > cur.ride.DescentM
		if prevAsc != curAsc {
			out = append(out, cur)
			continue
		}
		mergedStats := computeStats(rows, prev.startIdx, cur.endIdx, cfg)
		if !validateRide(mergedStats, prev.ride.Confidence, cfg) {
			out = append(out, cur)
			continue
		}
		dprintf("MERGE rides gap=%.1fs dist=%.1fm prev=%s..%s cur=%s..%s\n",
			gap, dist,
			prev.ride.StartedAt.Format("15:04:05"), prev.ride.EndedAt.Format("15:04:05"),
			cur.ride.StartedAt.Format("15:04:05"), cur.ride.EndedAt.Format("15:04:05"))
		out[len(out)-1] = rideWithIdx{
			ride: LiftRide{
				StartedAt:  prev.ride.StartedAt,
				EndedAt:    cur.ride.EndedAt,
				DurationS:  int(cur.ride.EndedAt.Sub(prev.ride.StartedAt).Seconds()),
				AscentM:    mergedStats.altPosDelta,
				DescentM:   mergedStats.altNegDelta,
				DistanceM:  mergedStats.cumDist,
				Confidence: prev.ride.Confidence,
			},
			startIdx: prev.startIdx,
			endIdx:   cur.endIdx,
		}
	}
	return out
}

// stats are the aggregate facts about a [start, end] window in the stream.
type stats struct {
	altPosDelta float64 // sum of positive per-pair altitude deltas
	altNegDelta float64 // sum of (positive-valued) negative per-pair altitude deltas
	altRateMs   float64 // (alt[end] - alt[start]) / duration  (signed)
	cumDist     float64 // path length (haversine sum)
	straight    float64 // start-to-end haversine
	sinuosity   float64 // cumDist / max(straight, 1)
	durationS   float64
	speeds      []float64
	speedsOK    bool
	meanSpeed   float64
	speedCV     float64 // trimmed
	altsOK      bool
}

func computeStats(rows []StreamPoint, start, end int, cfg DetectorConfig) stats {
	var s stats
	if end <= start || end >= len(rows) {
		return s
	}
	s.durationS = rows[end].Time.Sub(rows[start].Time).Seconds()

	// Altitude
	var firstAlt, lastAlt *float64
	prevAlt := math.NaN()
	altCount := 0
	for k := start; k <= end; k++ {
		if rows[k].AltitudeM == nil {
			continue
		}
		altCount++
		a := *rows[k].AltitudeM
		if firstAlt == nil {
			v := a
			firstAlt = &v
			prevAlt = a
			continue
		}
		v := a
		lastAlt = &v
		d := a - prevAlt
		if d > 0 {
			s.altPosDelta += d
		} else {
			s.altNegDelta += -d
		}
		prevAlt = a
	}
	if firstAlt != nil && lastAlt != nil && altCount >= 2 {
		s.altsOK = true
		s.altRateMs = (*lastAlt - *firstAlt) / math.Max(s.durationS, 1)
	}

	// Distance + sinuosity (haversine sum over geo-valid pairs; straight = first→last)
	var firstLat, firstLng, lastLat, lastLng *float64
	var prev *StreamPoint
	for k := start; k <= end; k++ {
		p := rows[k]
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		if firstLat == nil {
			la, ln := *p.Lat, *p.Lng
			firstLat, firstLng = &la, &ln
		}
		la, ln := *p.Lat, *p.Lng
		lastLat, lastLng = &la, &ln
		if prev != nil {
			s.cumDist += Haversine(*prev.Lat, *prev.Lng, *p.Lat, *p.Lng)
		}
		pp := p
		prev = &pp
	}
	if firstLat != nil && lastLat != nil {
		s.straight = Haversine(*firstLat, *firstLng, *lastLat, *lastLng)
	}
	s.sinuosity = s.cumDist / math.Max(s.straight, 1)

	// Chunked speeds: split the window into ~10 s slices, compute mean speed
	// per slice from haversine distance ÷ slice duration. This naturally
	// smooths per-sample GPS noise (a chair-bound bike reads SpeedMs=0 yet
	// GPS shows steady motion) while still discriminating MTB descents
	// whose chunk-to-chunk speed varies as the rider brakes/accelerates.
	const chunkSecs = 10.0
	chunked, _ := chunkedSpeeds(rows, start, end, chunkSecs)
	if len(chunked) > 0 {
		s.speeds = chunked
		s.speedsOK = true
		s.meanSpeed = mean(chunked)
		// Speed dispersion via robust MAD (median absolute deviation
		// over median) instead of stddev/mean CV. Computed over the
		// "core" chunks — drop the first and last chunk when at least 6
		// chunks are present. The first chunk captures the boarding
		// acceleration (cabin going from 0 to 8 m/s in 10 s on a
		// aerial tramway), the last chunk the disembark deceleration. Both
		// are real-but-unrepresentative outliers; including them rejects
		// long but legitimate aerial tramways (alpine bikepark mountain station case 1, 240 s,
		// raw cv=0.148 vs core cv=0.05).
		//
		// On top of boundary-trimming, rank-trim the top+bottom 10% of
		// chunks once the boundary-trimmed window holds ≥ 10 chunks.
		// Smartphone-grade GPS introduces single-chunk spikes (smartphone-GPS recording
		// mountain station case ride had a 4 m/s mid-window drop in an otherwise 7-8 m/s
		// lift) that alone pushed MAD/median from 0.040 to 0.054 and
		// rejected a real aerial tramway. The rank-trim defangs these without
		// relaxing the absolute MaxSpeedCV cap that protects against
		// MTB-pumptrack false positives.
		core := chunked
		if len(chunked) >= 6 {
			core = chunked[1 : len(chunked)-1]
			if len(core) >= 10 {
				sorted := append([]float64(nil), core...)
				sort.Float64s(sorted)
				trimN := int(math.Round(0.10 * float64(len(sorted))))
				if trimN > 0 {
					core = sorted[trimN : len(sorted)-trimN]
				}
			}
		}
		s.speedCV = robustDispersion(core)
	}
	return s
}

func robustDispersion(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	med := median(xs)
	if med <= 0 {
		return 0
	}
	devs := make([]float64, len(xs))
	for i, x := range xs {
		devs[i] = math.Abs(x - med)
	}
	return median(devs) / med
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// chunkedSpeeds slices [start, end] into chunks of approximately chunkSecs
// seconds (last chunk may be shorter, dropped if < chunkSecs/2 to avoid
// noise), and returns one mean speed per chunk computed from cumulative
// haversine distance. Returns the chunk count so callers can size CV
// expectations.
func chunkedSpeeds(rows []StreamPoint, start, end int, chunkSecs float64) ([]float64, int) {
	if end <= start {
		return nil, 0
	}
	var out []float64
	cs := start
	for k := start + 1; k <= end; k++ {
		elapsed := rows[k].Time.Sub(rows[cs].Time).Seconds()
		if elapsed < chunkSecs {
			continue
		}
		var dist float64
		for m := cs; m < k; m++ {
			if rows[m].Lat == nil || rows[m+1].Lat == nil {
				continue
			}
			dist += Haversine(*rows[m].Lat, *rows[m].Lng, *rows[m+1].Lat, *rows[m+1].Lng)
		}
		dt := rows[k].Time.Sub(rows[cs].Time).Seconds()
		if dt > 0 {
			out = append(out, dist/dt)
		}
		cs = k
	}
	// Trailing partial chunk: include only if at least chunkSecs/2 seconds long.
	if cs < end {
		dt := rows[end].Time.Sub(rows[cs].Time).Seconds()
		if dt >= chunkSecs/2 {
			var dist float64
			for m := cs; m < end; m++ {
				if rows[m].Lat == nil || rows[m+1].Lat == nil {
					continue
				}
				dist += Haversine(*rows[m].Lat, *rows[m].Lng, *rows[m+1].Lat, *rows[m+1].Lng)
			}
			out = append(out, dist/dt)
		}
	}
	return out, len(out)
}

func movingAverage(xs []float64, w int) []float64 {
	if len(xs) <= w {
		return xs
	}
	out := make([]float64, len(xs))
	half := w / 2
	for i := range xs {
		lo := i - half
		if lo < 0 {
			lo = 0
		}
		hi := i + half
		if hi >= len(xs) {
			hi = len(xs) - 1
		}
		var s float64
		for j := lo; j <= hi; j++ {
			s += xs[j]
		}
		out[i] = s / float64(hi-lo+1)
	}
	return out
}

func trimSpeeds(rows []StreamPoint, start, end int, speeds []float64, cfg DetectorConfig) []float64 {
	if cfg.SpeedCVTrimSeconds <= 0 {
		return speeds
	}
	totalDur := rows[end].Time.Sub(rows[start].Time).Seconds()
	usable := totalDur - 2*float64(cfg.SpeedCVTrimSeconds)
	if usable < 30 {
		return speeds
	}
	// Determine sample indices that lie inside [start+trim, end-trim] in time.
	trimStartT := rows[start].Time.Add(time.Duration(cfg.SpeedCVTrimSeconds)*time.Second)
	trimEndT := rows[end].Time.Add(-time.Duration(cfg.SpeedCVTrimSeconds)*time.Second)
	out := make([]float64, 0, len(speeds))
	idx := 0
	for k := start; k <= end; k++ {
		if _, ok := effectiveSpeedCfg(rows, k, cfg); !ok {
			continue
		}
		t := rows[k].Time
		if !t.Before(trimStartT) && !t.After(trimEndT) {
			out = append(out, speeds[idx])
		}
		idx++
	}
	if len(out) < 5 {
		return speeds
	}
	return out
}

func classify(ws stats, cfg DetectorConfig) string {
	if ws.sinuosity > cfg.MaxSinuosity {
		return ""
	}
	if !ws.altsOK {
		return ""
	}
	absRate := math.Abs(ws.altRateMs)
	if ws.speedsOK {
		mean := ws.meanSpeed
		// Only accept a candidate when speed is in lift band AND CV bounded.
		if mean < cfg.MinSpeedMs || mean > cfg.MaxSpeedMs {
			return ""
		}
		// Initial classify uses a relaxed CV cap. The 30 s initial window
		// holds only 3 chunks at 10 s each, so MAD/median is volatile —
		// boarding-acceleration plus one normal-running chunk often hits
		// MaxSpeedCV exactly. validateRide() applies the strict cap on
		// the extended-and-trimmed candidate downstream.
		cvCap := cfg.InitialMaxSpeedCV
		if cvCap == 0 {
			cvCap = cfg.MaxSpeedCV
		}
		if ws.speedCV >= cvCap {
			return ""
		}
		gradient := absRate / math.Max(mean, 0.001)
		// Ascending lifts only for the "high" branch — a clean
		// 60–90 s ski descent at constant speed and low sinuosity also
		// passes the gradient/CV gates, so we restrict "high" to the
		// going-up signature. (True downhill cable cars are rare and
		// will need an explicit OSM-anchored or sport-flag path; today
		// none of the training corpus contains one.)
		if ws.altRateMs >= 0.05 && gradient > cfg.MinGradient {
			return "high"
		}
		// Bidirectional fallback — straighter than the unidirectional bound.
		if ws.sinuosity <= cfg.BidirSinuosityMax {
			return "bidir"
		}
		return ""
	}
	// No usable speed → medium relies on altitude rate alone.
	if absRate > cfg.NoSpeedMinAltRateMs {
		return "medium"
	}
	return ""
}

// extendCandidate advances the window forward from initialEnd. The extension
// rule depends on the kind:
//
//   - high / medium — altitude direction continues (Python rule, ±2 m tolerance)
//                     plus stop-tolerance with altitude-reversal-on-resume check.
//   - bidir         — extend while effective speed stays in lift band; close on
//                     the same stop-tolerance + altitude-reversal-on-resume check.
//                     No per-step direction check (the whole point of bidir).
func extendCandidate(rows []StreamPoint, start, initialEnd int, kind string, ws stats, cfg DetectorConfig) int {
	end := initialEnd
	stopStartIdx := -1
	stopAccumS := 0.0
	goingUp := ws.altRateMs > 0
	// endViaStopIndicator: set when the loop terminates because the data
	// already encodes "the lift has stopped here" (cabin parked, plateau,
	// disembark, peak reversal). Used downstream to suppress PEAKADV /
	// trimTrailingLowGradient — those trims peel back past the bergstation
	// when the stop-indicator already pinned end correctly.
	endViaStopIndicator := false

	// peakAlt tracks running max (going up) or min (going down) so we can
	// close the window when altitude has fallen/risen far enough off the
	// peak — sustained reversal that per-pair `altDelta < -2` cannot catch
	// (skiing between lifts loses altitude at <1 m/s, never tripping the
	// per-pair 2 m threshold).
	//
	// peakStallMaxS guards against lift-top plateau over-extension: once the
	// peak altitude stops advancing for this many seconds, the lift has
	// effectively ended (rider has dismounted, even if speed has not yet
	// risen above MinSpeedMs).
	// peakDropThreshM = single-sample altitude drop off the peak that signals
	// the lift has ended. Originally 5 m, but ski-FIT barometers produce 5–9 m
	// single-sample wobble on a steadily climbing lift (multi-resort case 15:19:15
	// peak 1985 → 1978 → 1995 within 30 s). 12 m holds against the wobble
	// while still firing on real disembarks — stepping off the chair drops
	// alt 20–30 m within the first 30 s of skiing.
	const peakDropThreshM = 12.0
	const peakAdvanceMinM = 3.0 // robust to ±1 m barometer noise
	const stallTimeoutS = 20.0
	// stallMinDropM aligned with peakDropThreshM = 12 — both gate the same
	// physical event (sustained altitude reversal); the difference is only
	// the time aspect (stall = drop + 20 s without advance, peakDrop =
	// instant single-sample drop). 12 m holds against the 5–10 m ski-FIT
	// barometer wobble without losing real disembarks.
	const stallMinDropM = 12.0
	lastAlt := math.NaN()
	peakAlt := math.NaN()
	peakAdvanceAlt := math.NaN()
	peakAdvanceIdx := initialEnd
	lastAdvanceTime := rows[initialEnd].Time
	for k := initialEnd; k >= start; k-- {
		if rows[k].AltitudeM != nil {
			lastAlt = *rows[k].AltitudeM
			peakAdvanceIdx = k
			lastAdvanceTime = rows[k].Time
			break
		}
	}
	if !math.IsNaN(lastAlt) {
		peakAlt = lastAlt
		peakAdvanceAlt = lastAlt
	}

	// Bidir-only sinuosity guard: an aerial tramway track is a straight line, so
	// the cumulative-distance / straight-line ratio stays near 1.0 for the
	// entire ride. Without this check, a bidir window keeps growing when
	// the rider dismounts at the upper station and immediately rolls into
	// a bike-park trail (smartphone-GPS recording alpine bikepark 11:16: real bidirectional aerial tramway ~260 s,
	// extension ran 1577 s with sin=1.94). Probed every 30 indices to keep
	// the cost down — the rider's first off-lift turn pushes sin above the
	// 1.05 cap within a chunk. validateRide()'s final sinuosity check uses
	// the same cap; this just stops the window from over-extending into
	// non-lift terrain before the trim/validate stage runs.
	const bidirSinCheckEvery = 30
	bidirSinLastCheckEnd := initialEnd

	for end+1 < len(rows) {
		cur := rows[end]
		nxt := rows[end+1]
		dt := nxt.Time.Sub(cur.Time).Seconds()
		if dt <= 0 || dt > float64(cfg.SmartRecMaxGapS) {
			dprintf("BREAK gap dt=%.1f at end=%d t=%s\n", dt, end, cur.Time.Format("15:04:05"))
			break
		}
		spd, spdOK := effectiveSpeedCfg(rows, end+1, cfg)
		// Stop branch. Ambiguous (no usable speed) samples interleaved into
		// a real stop must not break the stop accumulation — at the
		// bergstation GPS often drops out for 1–6 s while the cabin is
		// stationary, and a 30 s disembark would otherwise be split into
		// 5–11 s sub-stops. While already in a stop, we stay in the stop
		// until a clear moving sample arrives (spdOK && spd >= MinSpeed).
		var isStop bool
		if stopStartIdx != -1 {
			isStop = !(spdOK && spd >= cfg.MinSpeedMs)
		} else {
			isStop = spdOK && spd < cfg.MinSpeedMs
		}
		if isStop {
			if stopStartIdx == -1 {
				stopStartIdx = end
				dprintf("STOP enter at end=%d t=%s spd=%.2f\n", end, cur.Time.Format("15:04:05"), spd)
			}
			stopAccumS += dt
			if stopAccumS > float64(cfg.StopToleranceS) {
				dprintf("BREAK stop overflow %.1fs\n", stopAccumS)
				end = stopStartIdx
				endViaStopIndicator = true
				break
			}
			end++
			continue
		}
		// Resume from stop
		if stopStartIdx != -1 {
			if shouldCloseOnResume(rows, start, stopStartIdx, end+1, cfg) {
				dprintf("BREAK resume reversal at stopStart=%d resume=%d\n", stopStartIdx, end+1)
				end = stopStartIdx
				endViaStopIndicator = true
				break
			}
			// Disembark vs pylon-stop: a real lift-end stop has the rider
			// stepping off and rolling/walking on a near-flat plateau —
			// post-resume altitude rate stays well below pre-stop rate. A
			// pylon stop interrupts the same cable run, so post-resume rate
			// recovers to the pre-stop level. DB cross-check on
			// mtb-park case-2 ride[3] (14:45:27→14:45:39): 12 s stop, pre-stop
			// rate 0.55 m/s on the chairlift cable, post-resume 0.05 m/s
			// crossing the bergstation plateau — ratio 0.09. Cap at 0.40
			// to keep variability in real lift cables (cabin-brake regions
			// drift to 0.5 × cable rate at most).
			if (kind == "high" || kind == "medium") && stopStartIdx >= 0 {
				stopDur := rows[end+1].Time.Sub(rows[stopStartIdx].Time).Seconds()
				if stopDur >= 8.0 {
					// Pre-stop window 60 s: at the bergstation cable cars decelerate
					// for the last 15–30 s, so a 30 s window already shows half the
					// disembark drift (alpine ski-resort case ride[01]: pre-30s rate 0.20 m/s
					// even though the cable was running 0.7+ m/s through the
					// preceding minute). 60 s averages over enough cable to hold
					// pre-stop rate above the 0.30 m/s lift floor while the brake
					// drift dilutes only to ~0.32 m/s.
					preStopRate := altRateLastSeconds(rows, start, stopStartIdx, 60.0)
					postResumeRate := altRateNextSeconds(rows, end+1, 30.0, len(rows)-1)
					// Pre-stop floor 0.40 m/s ensures we are clearly on a lift
					// cable (chairlifts run 0.5–2 m/s vertical, gondolas 1–3.8;
					// boarding-zone shuffles sit at 0.1–0.3 even with a mild
					// alt drift). Ratio cap 0.30 keeps the test sharp: a true
					// disembark drops to 0.05–0.15 m/s of skating, while a
					// resuming pylon stop returns to >0.5 × pre-stop rate.
					if preStopRate >= 0.40 && postResumeRate < 0.30*preStopRate {
						dprintf("BREAK disembark stopDur=%.1fs preRate=%.3f postRate=%.3f at stopStart=%d\n",
							stopDur, preStopRate, postResumeRate, stopStartIdx)
						end = stopStartIdx
						endViaStopIndicator = true
						break
					}
				}
			}
			dprintf("RESUME ok stopAccum=%.1fs stopStart=%d\n", stopAccumS, stopStartIdx)
			stopStartIdx = -1
			stopAccumS = 0
		}
		// Out-of-band speed → close. The threshold is intentionally above
		// MaxSpeedMs (which validateRide enforces against the mean): a
		// single sample-level GPS speed spike (alpine bikepark mountain station case 1
		// aerial tramway at 08:58:45: 17.8 m/s instantaneous through a
		// continuously-moving cabin) must not break a real ride. The
		// chunk-mean and median CV gates downstream still reject true
		// over-band candidates.
		const overBandBreakSpdMs = 22.0
		if spdOK && spd > overBandBreakSpdMs {
			dprintf("BREAK over-band spd=%.1f at end=%d t=%s\n", spd, end, cur.Time.Format("15:04:05"))
			break
		}
		// 60-second flat-plateau check: if altitude has barely changed
		// over the past 60 s the lift is stationary or the rider has
		// reached the top — close the window. This is a backstop that
		// catches mid-tour station-to-station transfers (e.g. transfer case
		// transfer-station case → secondary transfer station at 1334 m for 4 minutes) where
		// altitude stays exactly flat so the drop-based stall does not
		// fire, AND boarding-zone idle time longer than 60 s. Disabled
		// for bidir: an aerial tramway's valley crossing (bidirectional aerial tramway at alpine bikepark:
		// 14:17:06 alt=2446 → 14:18:06 alt=2444) looks identical to a
		// plateau over 60 s; the trim-back-to-lastInBandIdx below handles
		// the trailing disembark zone instead.
		if (kind == "high" || kind == "medium") && nxt.AltitudeM != nil {
			// plateauWinS raised from 60 → 65 s. Mid-lift Stops up to ~50 s
			// (mtb-park case 13:51 ~30 s lift halt, multi-resort case 15:29 mid-chairlift
			// tower / pylon wobble) leave the 65 s window dominated by surrounding
			// climb-segments and stay above the 2 m delta cap. Real station-
			// to-station transfers — transfer case transfer-station case → secondary transfer station
			// (~4 min walk), Suunto alpine bikepark (~83 s inter-lift transfer),
			// multi-section case bikepark transfer casel-Anschluss (~3 min) — all stretch alt-flat
			// past the 65 s window and still trip the check, so the two lifts
			// are recorded as separate rides.
			const plateauWinS = 65.0
			const plateauMaxDeltaM = 2.0
			cutoff := nxt.Time.Add(-time.Duration(plateauWinS) * time.Second)
			var refAlt float64 = math.NaN()
			refOK := false
			for k := end; k >= start; k-- {
				if rows[k].Time.Before(cutoff) {
					break
				}
				if rows[k].AltitudeM != nil {
					refAlt = *rows[k].AltitudeM
					refOK = true
				}
			}
			if refOK && nxt.Time.Sub(rows[start].Time).Seconds() >= plateauWinS {
				if math.Abs(*nxt.AltitudeM-refAlt) < plateauMaxDeltaM {
					dprintf("BREAK plateau60s ref=%.1f cur=%.1f at end=%d t=%s\n",
						refAlt, *nxt.AltitudeM, end, cur.Time.Format("15:04:05"))
					endViaStopIndicator = true
					break
				}
			}
		}

		// Direction continuation for high/medium — peak-reversal only.
		// Per-pair altDelta is too sensitive to barometer sample noise
		// (Garmin streams routinely show ±2-3m single-sample jitter on a
		// steadily climbing gondola); peakDropThreshM=5m catches sustained
		// reversals robustly.
		if (kind == "high" || kind == "medium") && nxt.AltitudeM != nil && !math.IsNaN(lastAlt) {
			// PeakDrop / stall do NOT set endViaStopIndicator: those breaks
			// fire AFTER alt has reversed >= peakDropThreshM (12 m) off the
			// peak — end is already 12 m into the disembark descent / next-
			// ride trail. The trims still need to walk back to the actual
			// peak. Setting the flag here would leak skiing/biking samples
			// into the ride (multi-section case 1 ride[06-08]: desc=46-63 m mixed
			// into asc=89-128 m — the descent leaked from the peak-drop
			// boundary into the ride window).
			if goingUp {
				if !math.IsNaN(peakAlt) && peakAlt-*nxt.AltitudeM > peakDropThreshM {
					dprintf("BREAK peakDrop peak=%.1f cur=%.1f at end=%d t=%s\n", peakAlt, *nxt.AltitudeM, end, cur.Time.Format("15:04:05"))
					break
				}
				// Peak-stall: no upward advance for stallTimeoutS AND alt
				// has dropped at least stallMinDropM off the peak. The drop
				// requirement keeps mid-ride plateaus (gondola dwell at a
				// turn-around / boarding-zone slowdown) from triggering a
				// false close — those leave alt stagnant, not lower.
				stalled := nxt.Time.Sub(lastAdvanceTime).Seconds() > stallTimeoutS
				dropped := !math.IsNaN(peakAlt) && peakAlt-*nxt.AltitudeM >= stallMinDropM
				if stalled && dropped {
					dprintf("BREAK stall lastAdv=%s peak=%.1f cur=%.1f at end=%d\n",
						lastAdvanceTime.Format("15:04:05"), peakAlt, *nxt.AltitudeM, end)
					break
				}
			} else {
				if !math.IsNaN(peakAlt) && *nxt.AltitudeM-peakAlt > peakDropThreshM {
					dprintf("BREAK peakRise at end=%d\n", end)
					break
				}
				stalled := nxt.Time.Sub(lastAdvanceTime).Seconds() > stallTimeoutS
				risen := !math.IsNaN(peakAlt) && *nxt.AltitudeM-peakAlt >= stallMinDropM
				if stalled && risen {
					dprintf("BREAK stall(down) lastAdv=%s\n", lastAdvanceTime.Format("15:04:05"))
					break
				}
			}
		}
		if kind == "bidir" && (end-bidirSinLastCheckEnd) >= bidirSinCheckEvery {
			sin := windowSinuosity(rows, start, end)
			if sin > cfg.BidirSinuosityMax {
				dprintf("BREAK bidir-sin sin=%.3f at end=%d t=%s\n",
					sin, end, rows[end].Time.Format("15:04:05"))
				break
			}
			bidirSinLastCheckEnd = end
		}
		if nxt.AltitudeM != nil {
			lastAlt = *nxt.AltitudeM
			if math.IsNaN(peakAlt) {
				peakAlt = *nxt.AltitudeM
				peakAdvanceAlt = *nxt.AltitudeM
				peakAdvanceIdx = end + 1
				lastAdvanceTime = nxt.Time
			} else if goingUp {
				if *nxt.AltitudeM > peakAlt {
					peakAlt = *nxt.AltitudeM
				}
				if *nxt.AltitudeM > peakAdvanceAlt+peakAdvanceMinM {
					peakAdvanceAlt = *nxt.AltitudeM
					peakAdvanceIdx = end + 1
					lastAdvanceTime = nxt.Time
				}
			} else {
				if *nxt.AltitudeM < peakAlt {
					peakAlt = *nxt.AltitudeM
				}
				if *nxt.AltitudeM < peakAdvanceAlt-peakAdvanceMinM {
					peakAdvanceAlt = *nxt.AltitudeM
					peakAdvanceIdx = end + 1
					lastAdvanceTime = nxt.Time
				}
			}
		}
		end++
	}
	// Trim end back to the last sample where altitude actually advanced — any
	// trailing plateau samples (rider waiting at lift top) only inflate the
	// stats with sensor-zero / GPS-noisy speed readings.
	// PeakAdvance trim only when the loop ran out without a stop-indicator
	// break: those breaks (disembark, plateau60s, stop overflow, resume
	// reversal) already pin end at the bergstation arrival. peakAdvanceMinM=3
	// uses strict-greater-than against the running peak, so a final 773→776
	// alt step at the bergstation does not register and leaves
	// peakAdvanceIdx pinned 30 s earlier — slow-cable bikepark case ride[02] @ 11:40:01
	// (disembark BREAK) was cut by this. The stop-indicator gate fixes that
	// while keeping the trim active for natural-end candidates that just hit
	// stream end / a smart-recording gap.
	if (kind == "high" || kind == "medium") && peakAdvanceIdx > start && peakAdvanceIdx < end && !endViaStopIndicator {
		end = peakAdvanceIdx
	}
	// Trailing trim (per-kind selection):
	//   - Stop-indicator end + fast lift (rideAltRate ≥ 0.30 m/s): trim only
	//     the truly stationary tail samples (speed < MinSpeedMs). The BREAK
	//     already pinned end at stopStartIdx (FIRST low-speed sample); the
	//     10–30 s of cable-decel just before it is part of the lift and must
	//     be preserved. Chunk-trim removes those decel chunks because their
	//     GPS-haversine speed (1.5–2 m/s) falls below 0.5 × median (1.95 m/s
	//     for cruise=3.9), landing 30–65 m short of the bergstation. alpine ski-resort case
	//     ride[03] (rideAltRate=0.92, BREAK disembark at 11:15:33, chunk-trim
	//     cuts to 11:15:04 = 63 m short) is the calibrated case. Sample-level
	//     trim walks back only while speed < MinSpeedMs, keeping the entire
	//     decel zone in the ride.
	//   - All other endings (slow chairlift OR natural-end): keep the chunk
	//     trim. Slow chairlifts (slow-cable bikepark case Bahn 2, rideAltRate=0.20) need it
	//     because BREAK fires LATE (preStopRate ≥ 0.40 discriminator unmet on
	//     0.2 m/s cable) and end runs into 80+ s of bergstation walk-around;
	//     chunk-trim correctly peels that back. Natural-end candidates rely
	//     on it for aerial-tramway disembark coast (8 m/s → walk pace in 30 s).
	rideDur := rows[end].Time.Sub(rows[start].Time).Seconds()
	rideAltRate := 0.0
	if rideDur > 0 && rows[start].AltitudeM != nil && rows[end].AltitudeM != nil {
		rideAltRate = math.Abs(*rows[end].AltitudeM-*rows[start].AltitudeM) / rideDur
	}
	const fastLiftAltRate = 0.30
	if endViaStopIndicator && rideAltRate >= fastLiftAltRate {
		end = trimTrailingLowSpeedSamples(rows, start, end, cfg)
		// Walk-off trim: at fast-lift bergstations the cable's altitude
		// plateaus 5–15 s before the rider disembarks, then the rider walks
		// 10–35 m past the cable terminal at 1–3 m/s before BREAK disembark
		// fires on the gate-stop. trimTrailingLowSpeedSamples does not catch
		// it (walking speed is above MinSpeedMs=0.7), so end overshoots the
		// bergstation by 30+ m on stations with longer disembark walks.
		// multi-resort case r9 (Lift 4): 34 m past, r13 (Lift 8): 31 m past.
		// Walk back while the trailing 5 s alt-rate at end is below 0.15 m/s
		// — well under fast-lift cruise (0.30+ m/s) and under the natural
		// cable-decel slope (0.20–0.40 m/s on alpine ski-resort case-class gondolas), so
		// only the truly flat platform tail gets peeled.
		end = trimTrailingAltPlateau(rows, start, end)
	} else {
		end = trimTrailingChunks(rows, start, end)
	}
	// Trailing-sinuosity trim: speed-based trim above misses bike-park trails
	// that descend at lift-comparable speeds (5–10 m/s) but curve sharply —
	// alpine ski-resort case ride[01] end[1] runs the lift-cable straight, then keeps going
	// down a switchback ski/wander trail; mtb-park case ride[7], mtb-park case-2 ride[3]
	// end[3] do the same on bike-park downhill flow. Probe the tail in 30 s
	// windows: while the tail's sinuosity > 1.20, peel back 10 s and re-check.
	// Skip for bidir (the per-loop bidir-sin guard already handles it).
	end = trimTrailingHighSinuosity(rows, start, end, kind)
	// (disembark-discontinuity test was tried and reverted: peel-back logic
	// false-triggered on mid-lift pylon stops, causing 5+ phantom lifts in
	// multi-resort case/multi-section case. Pylon dwell creates the same mid-vs-tail ratio
	// signature as a real disembark. Solving that needs lookahead — checking
	// whether altRate recovers — which the post-trim layer lacks. Disembark
	// detection happens in the forward pass via peakStall + plateau60s.)
	// Trailing-low-gradient trim: catches the post-disembark uphill skating /
	// flat schuss that the discontinuity test misses because the mid-window
	// itself has already dropped below 0.30 m/s. Probe last 60 s; if its
	// gradient + altRate stay below the lift floor, peel back 10 s and retest.
	end = trimTrailingLowGradient(rows, start, end, kind, cfg)
	return end
}

// trimTrailingDisembarkDiscontinuity searches the last 3 minutes of the ride
// for the altitude-rate step that marks lift-end-to-disembark. Slides pos
// backward in 10 s steps; at each pos compares 30 s tail vs preceding 60 s
// mid window. When mid is firmly in lift band (≥ 0.30 m/s) AND tail collapses
// to ≤ 25 % of mid, the disembark step is at midEnd — single trim, return.
// midAltRateFloor must be lower than typical cable speeds so we find the
// step even on slow drag-lifts; tailRatioMax stays well below 0.5 so a
// genuine cabin-brake (0.5 m/s tail vs 1.0 m/s mid → ratio 0.50) is kept.
func trimTrailingDisembarkDiscontinuity(rows []StreamPoint, start, end int, kind string) int {
	if kind == "bidir" {
		return end
	}
	const midWindowS = 60.0
	const tailWindowS = 30.0
	const stepS = 10.0
	const maxScanBackS = 180.0 // never scan more than 3 min from initial end
	const midAltRateFloor = 0.30
	const tailRatioMax = 0.25
	scanLimitT := rows[end].Time.Add(-time.Duration(maxScanBackS) * time.Second)
	pos := end
	for pos > start+30 {
		if rows[pos].Time.Before(scanLimitT) {
			return pos
		}
		tailEndT := rows[pos].Time
		tailStartT := tailEndT.Add(-time.Duration(tailWindowS) * time.Second)
		midEndT := tailStartT
		midStartT := midEndT.Add(-time.Duration(midWindowS) * time.Second)
		if midStartT.Before(rows[start].Time) {
			return pos
		}
		findIdxAfter := func(t time.Time) int {
			for k := pos; k > start; k-- {
				if rows[k].Time.Before(t) {
					return k + 1
				}
			}
			return start
		}
		tailStart := findIdxAfter(tailStartT)
		midStart := findIdxAfter(midStartT)
		midEnd := findIdxAfter(midEndT) - 1
		if midEnd < midStart || tailStart > pos {
			return pos
		}
		midRate := altitudeRate(rows, midStart, midEnd)
		tailRate := altitudeRate(rows, tailStart, pos)
		// Discontinuity criterion: clear lift mid AND tail collapse.
		if midRate >= midAltRateFloor && tailRate < tailRatioMax*midRate {
			dprintf("TAIL-DISC mid=%.3f tail=%.3f ratio=%.2f trim pos %d->%d\n",
				midRate, tailRate, tailRate/midRate, pos, midEnd)
			return midEnd
		}
		// Either mid not yet at lift level (whole window post-disembark) or
		// tail still lift-like (haven't reached the step). Peel back 10 s
		// and retry. Never returns "ride extended" — only same or shorter.
		cutBackT := rows[pos].Time.Add(-time.Duration(stepS) * time.Second)
		newPos := pos
		for k := pos - 1; k > start; k-- {
			if !rows[k].Time.After(cutBackT) {
				newPos = k
				break
			}
		}
		if newPos >= pos {
			return pos
		}
		pos = newPos
	}
	return pos
}

// altRateLastSeconds returns |Δalt|/duration over the last `seconds` of the
// range [lo, hi]. lo is the lower bound (typically the ride's start; never
// search past it). Returns 0 if there are not enough altitude-bearing samples.
func altRateLastSeconds(rows []StreamPoint, lo, hi int, seconds float64) float64 {
	if hi <= lo || hi >= len(rows) {
		return 0
	}
	cutoff := rows[hi].Time.Add(-time.Duration(seconds) * time.Second)
	startIdx := lo
	for k := hi; k > lo; k-- {
		if rows[k].Time.Before(cutoff) {
			startIdx = k + 1
			break
		}
	}
	return altitudeRate(rows, startIdx, hi)
}

// altRateNextSeconds returns |Δalt|/duration over the next `seconds` of the
// range starting at lo. hi is the upper bound (last valid stream index).
// Returns 0 if there are not enough altitude-bearing samples or the range
// runs out before reaching `seconds` of data.
func altRateNextSeconds(rows []StreamPoint, lo int, seconds float64, hi int) float64 {
	if lo >= hi || lo < 0 || hi >= len(rows) {
		return 0
	}
	cutoff := rows[lo].Time.Add(time.Duration(seconds) * time.Second)
	endIdx := hi
	for k := lo; k <= hi; k++ {
		if rows[k].Time.After(cutoff) {
			endIdx = k - 1
			break
		}
	}
	if endIdx <= lo {
		return 0
	}
	return altitudeRate(rows, lo, endIdx)
}

// altitudeRate returns |alt[end] - alt[start]| / duration (in m/s) for the
// first/last altitude-bearing samples in [start, end]. Returns 0 if either
// boundary lacks altitude or duration < 1 s.
func altitudeRate(rows []StreamPoint, start, end int) float64 {
	if end <= start {
		return 0
	}
	var firstAlt, lastAlt *float64
	for k := start; k <= end; k++ {
		if rows[k].AltitudeM == nil {
			continue
		}
		if firstAlt == nil {
			v := *rows[k].AltitudeM
			firstAlt = &v
		}
		v := *rows[k].AltitudeM
		lastAlt = &v
	}
	if firstAlt == nil || lastAlt == nil {
		return 0
	}
	dur := rows[end].Time.Sub(rows[start].Time).Seconds()
	if dur < 1 {
		return 0
	}
	return math.Abs(*lastAlt-*firstAlt) / dur
}

// trimTrailingLowGradient walks end backward in 10 s steps while the trailing
// 60 s window's vertical-to-horizontal speed ratio stays under
// 0.5 × cfg.MinGradient OR its altRate stays under altRateFloor.
// Distinguishes a real lift's steady climb from a disembark-zone schuss /
// skating push / bergstation walk-around.
//
// altRateFloor is adaptive: max(absoluteFloor, 0.5 × ride-altRate). Slow
// chairlifts (slow-cable bikepark case Bahn 2: ride altRate ≈ 0.19 m/s; tail when on
// cable still ≈ 0.15 m/s) need a lower floor than fast gondolas (altRate
// ≥ 0.5 m/s). Anchoring to half the ride-altRate keeps the floor below
// the natural cable rate — the trim then only fires once the tail collapses
// to clearly-not-on-cable rates (bergstation walking ≈ 0.0 m/s, schuss off
// disembark ≈ 0.05 m/s). Absolute floor 0.05 m/s prevents the gate from
// vanishing on edge-case low-altRate rides.
//
func trimTrailingLowGradient(rows []StreamPoint, start, end int, kind string, cfg DetectorConfig) int {
	if kind == "bidir" {
		return end
	}
	const tailWindowS = 60.0
	const stepS = 10.0
	gradFloor := 0.5 * cfg.MinGradient
	// Compute the candidate's overall altRate so the floor scales with the
	// natural lift class. altDelta uses end-start direction-agnostic absolute.
	rideDur := rows[end].Time.Sub(rows[start].Time).Seconds()
	rideAltRate := 0.0
	if rideDur > 0 && rows[start].AltitudeM != nil && rows[end].AltitudeM != nil {
		rideAltRate = math.Abs(*rows[end].AltitudeM-*rows[start].AltitudeM) / rideDur
	}
	pos := end
	for pos > start+10 {
		cutoffT := rows[pos].Time.Add(-time.Duration(tailWindowS) * time.Second)
		if cutoffT.Before(rows[start].Time) {
			return pos
		}
		tailStart := -1
		for k := pos; k > start; k-- {
			if rows[k].Time.Before(cutoffT) {
				tailStart = k + 1
				break
			}
		}
		if tailStart < 0 || tailStart >= pos-2 {
			return pos
		}
		tailDur := rows[pos].Time.Sub(rows[tailStart].Time).Seconds()
		if tailDur < 30 {
			return pos
		}
		var firstAlt, lastAlt *float64
		for k := tailStart; k <= pos; k++ {
			if rows[k].AltitudeM == nil {
				continue
			}
			if firstAlt == nil {
				v := *rows[k].AltitudeM
				firstAlt = &v
			}
			v := *rows[k].AltitudeM
			lastAlt = &v
		}
		if firstAlt == nil || lastAlt == nil {
			return pos
		}
		altRate := math.Abs(*lastAlt-*firstAlt) / tailDur
		var sumSpd, cntSpd float64
		for k := tailStart; k <= pos; k++ {
			if spd, ok := effectiveSpeedCfg(rows, k, cfg); ok {
				sumSpd += spd
				cntSpd++
			}
		}
		if cntSpd < 5 {
			return pos
		}
		meanSpd := sumSpd / cntSpd
		if meanSpd < 0.001 {
			return pos
		}
		gradient := altRate / meanSpd
		// Two-strikes trim: ride continues only if BOTH gradient AND altitude
		// rate clear the lift floor. Floor scales with rideAltRate (slow
		// chairlifts run terminal cable at ~0.15 m/s — fixed 0.15 absolute
		// over-trims them). Absolute floor 0.05 keeps the gate active for
		// edge-case low-altRate rides where 0.5 × rideAltRate falls below
		// disembark schuss (~0.05–0.10 m/s).
		altRateFloor := 0.5 * rideAltRate
		if altRateFloor < 0.05 {
			altRateFloor = 0.05
		}
		if gradient >= gradFloor && altRate >= altRateFloor {
			return pos
		}
		// Cable-continuation bypass: a multi-section gondola (multi-section case
		// "Arena Express" Tal→Mid1→Mid2→Berg) holds a quasi-horizontal cable
		// segment for 60–120 s mid-ride, dropping tail altRate to 0.03–0.10
		// m/s while the cabin still glides on the cable. The two-strikes
		// gate then peels back through that segment until it meets the
		// preceding steep phase. Geometric signature distinguishes mid-cable
		// from a true bergstation/disembark tail:
		//   - sinuosity stays at 1.00–1.02 (pylon-straight cable line); a
		//     walk-around / disembark-schuss/skating departure produces
		//     ≥ 1.05 within 60 s.
		//   - speed CV stays low. Cable cruise is steady within the 60 s
		//     window even when the cable itself decelerates for boarding
		//     turbulence; walk-off collapses speed to walking pace and the
		//     cable-decel into bergstation halt drops speed across the
		//     window. CV ≤ 0.05 retains both classic gondolas and slow
		//     chairlifts (slow-cable bikepark case Bahn 2 cable cruise CV ≈ 0.03).
		// Both gates must pass — sinuosity alone misses linear walk-offs
		// and CV alone misses slow walks on the cable line.
		tailSinOK := false
		if rows[tailStart].Lat != nil && rows[tailStart].Lng != nil &&
			rows[pos].Lat != nil && rows[pos].Lng != nil {
			cumDist := 0.0
			for k := tailStart + 1; k <= pos; k++ {
				if rows[k-1].Lat == nil || rows[k-1].Lng == nil ||
					rows[k].Lat == nil || rows[k].Lng == nil {
					continue
				}
				cumDist += Haversine(*rows[k-1].Lat, *rows[k-1].Lng, *rows[k].Lat, *rows[k].Lng)
			}
			straight := Haversine(*rows[tailStart].Lat, *rows[tailStart].Lng, *rows[pos].Lat, *rows[pos].Lng)
			if straight > 1.0 && cumDist/straight <= 1.02 {
				tailSinOK = true
			}
		}
		if tailSinOK {
			var sumSqDev float64
			for k := tailStart; k <= pos; k++ {
				if spd, ok := effectiveSpeedCfg(rows, k, cfg); ok {
					d := spd - meanSpd
					sumSqDev += d * d
				}
			}
			cv := math.Sqrt(sumSqDev/cntSpd) / meanSpd
			if cv <= 0.05 {
				dprintf("TAIL-GRAD bypass cable-cont sin≈cable cv=%.3f tail=[%d..%d] pos=%d\n",
					cv, tailStart, pos, pos)
				return pos
			}
		}
		cutBackT := rows[pos].Time.Add(-time.Duration(stepS) * time.Second)
		newPos := pos
		for k := pos - 1; k > start; k-- {
			if !rows[k].Time.After(cutBackT) {
				newPos = k
				break
			}
		}
		if newPos >= pos {
			return pos
		}
		dprintf("TAIL-GRAD trim grad=%.3f<floor=%.3f tail=[%d..%d] pos %d->%d\n",
			gradient, gradFloor, tailStart, pos, pos, newPos)
		pos = newPos
	}
	return pos
}

// trimTrailingHighSinuosity walks the end backward in 10 s steps while the
// trailing 30 s window's sinuosity stays above sinTrimThresh. Stops when
// either the tail is straight again, or the window can no longer be measured
// (start of the ride approached). Skips bidir kinds because bidir lifts have
// per-loop sinuosity gating in extendCandidate.
//
// sinTrimThresh = 1.10: the bergstation-arrival walk-around (rider stepping
// off, circling 5–10 m to clear the lift exit) adds 10–30 curving samples
// that push the trailing 30 s sinuosity to 1.08–1.12. With the old 1.20 cap
// these samples leaked past the trim and pushed final ws.sinuosity above
// MaxSinuosity=1.08 — slow-cable bikepark case ride[05] @ 12:27:54 and ride[13] @ 14:53:12
// failed validateRide on sin=1.09. 1.10 strips the walking samples without
// false-trimming chairlifts whose cable curves slightly (Bahn 1 mid-cable
// sin = 1.005, well clear).
func trimTrailingHighSinuosity(rows []StreamPoint, start, end int, kind string) int {
	if kind == "bidir" {
		return end
	}
	const tailWindowS = 30.0
	const stepS = 10.0
	const sinTrimThresh = 1.10
	pos := end
	for pos > start+5 {
		cutoffT := rows[pos].Time.Add(-time.Duration(tailWindowS) * time.Second)
		if cutoffT.Before(rows[start].Time) {
			return pos
		}
		tailStart := -1
		for k := pos; k > start; k-- {
			if rows[k].Time.Before(cutoffT) {
				tailStart = k + 1
				break
			}
		}
		if tailStart < 0 || tailStart >= pos-2 {
			return pos
		}
		sin := windowSinuosity(rows, tailStart, pos)
		if sin <= sinTrimThresh {
			return pos
		}
		cutBackT := rows[pos].Time.Add(-time.Duration(stepS) * time.Second)
		newPos := pos
		for k := pos - 1; k > start; k-- {
			if !rows[k].Time.After(cutBackT) {
				newPos = k
				break
			}
		}
		if newPos >= pos {
			return pos
		}
		dprintf("TAIL-SIN trim sin=%.3f tail=[%d..%d] pos %d->%d\n", sin, tailStart, pos, pos, newPos)
		pos = newPos
	}
	return pos
}

// extendCandidateBackward walks start toward lowerBoundIdx (exclusive of any
// previous ride's samples) using the same stop set as the forward pass,
// inverted. Returns the extended start (≤ oldStart). Caller validates the
// extended window with computeStats + validateRide and discards the
// extension if it no longer passes — that guard keeps pre-boarding
// bike-track segments out of the ride.
//
// Stop conditions when walking from oldStart backward:
//   - time gap > SmartRecMaxGapS (recording was paused)
//   - speed under MinSpeedMs accumulating past StopToleranceS (boarding-zone
//     idle leading into the lift line; we pin start at the moment the rider
//     stopped moving so the ride begins at the boarding-line entry, not at
//     the parking lot)
//   - speed over overBandBreakSpdMs (instantaneous GPS spike outside lift
//     band — in backward direction this typically means a free-rolling bike
//     descent on a bike-park trail before entering the lift line)
//   - altitude reversal: for an ascending ride, alt rising > peakDropThreshM
//     above the original ride's start altitude; for a descending ride, the
//     mirror. The ride start anchors the "peak" — anything past it on the
//     wrong side belongs to a different segment.
//   - 65-second altitude plateau (high/medium only): if the altitude has not
//     advanced toward the ride direction for plateauWinS, we have walked
//     into the boarding-zone idle and must stop. Mirrors extendCandidate's
//     forward 65-s plateau check.
func extendCandidateBackward(rows []StreamPoint, oldStart, end int, kind string, ws stats, cfg DetectorConfig, lowerBoundIdx int) int {
	if oldStart <= 0 || oldStart <= lowerBoundIdx {
		return oldStart
	}
	start := oldStart
	goingUp := ws.altRateMs > 0

	// initialAlt anchors the "ride-direction peak" — any backward sample with
	// altitude past this anchor by more than peakDropThreshM has crossed into
	// non-lift terrain.
	initialAlt := math.NaN()
	for k := oldStart; k <= end; k++ {
		if rows[k].AltitudeM != nil {
			initialAlt = *rows[k].AltitudeM
			break
		}
	}

	const peakDropThreshM = 12.0
	const overBandBreakSpdMs = 22.0
	// backwardStopToleranceS is intentionally tighter than the forward
	// cfg.StopToleranceS (120 s). The forward pass tolerates long mid-lift
	// halts (mtb-park case 30 s tower-stop, multi-resort case pylon wobble). Backward,
	// the same tolerance lets the start drift through the boarding-line
	// queue (skiers shuffle 50–100 m before the gondola at < 0.5 m/s for
	// upwards of a minute) — exactly the boarding-plateau case "start before gondola"
	// shape. 30 s lets us absorb a mid-lift stop that bridges into the
	// upper part of the same ride while shielding boarding-queue drift.
	const backwardStopToleranceS = 30.0
	// altAdvanceStallBackS: walking backward, every prv sample whose
	// altitude does not move further "below" the running min (for an
	// ascending ride) increments a stall counter. After altAdvanceStallBackS
	// consecutive seconds without altitude advance, we are in the boarding
	// queue — alt-flat horizontal motion that the speed-stop branch never
	// catches because skiers shuffle at 0.8–1.2 m/s, well above MinSpeedMs.
	// 45 s tolerates mid-lift altitude wobble (multi-resort case 15:29 ride[10]:
	// barometer + GPS oscillates 2106→2121→2113 over 30 s while the cable
	// is steadily climbing) and pylon dwell/cabin-tilt micro-flats. The
	// boarding-plateau case (boarding-plateau case, transfer case transfer) takes well over
	// 60 s of strict alt-flat — those still trip the gate. validateRide
	// (with the relaxed CV cap on the backward extension) is the
	// downstream check that keeps over-reach in line.
	const altAdvanceStallBackS = 45.0
	const altAdvanceMinM = 3.0

	stopAccumS := 0.0
	stopStartIdx := -1
	// extremeAlt = lowest seen so far walking back when goingUp; highest
	// when descending. lastAdvanceTime = wall-clock of the last sample whose
	// altitude pushed extremeAlt past altAdvanceMinM in the ride direction.
	// lastAdvanceIdx = index of that same sample, used to pin start back to
	// the true boarding-to-lift boundary on alt-stall break (otherwise the
	// stall-break's `start` lands inside the boarding plateau and pulls
	// 30–180 s of low-speed queue-shuffle into the window — those samples
	// then explode SpeedCV past validateRide's 0.04 cap and the entire
	// extension is discarded by the caller).
	extremeAlt := initialAlt
	lastAdvanceTime := rows[oldStart].Time
	lastAdvanceIdx := oldStart

	// Sinuosity guard for the freshly-extended slice. Boarding queues
	// (skiers shuffling at the gondola entrance) and bike-park access trails
	// have markedly higher curvature than the lift cable itself. We probe
	// every backwardSinCheckEvery samples (≈ 30 s at 1 Hz, the dominant
	// sample rate); the slice [start-1, sliceTopIdx] captures only the just-
	// added samples, so a queue/trail entry shows up as a sharp ratio jump
	// that the global window-sinuosity (averaged over the whole ride) would
	// dilute. Threshold backwardSinMiniMax = 1.20 sits above MaxSinuosity
	// (1.08) so straight mid-lift segments stay intact, but below the 1.30+
	// values typical of queue zigzag.
	const backwardSinCheckEvery = 30
	const backwardSinMiniMax = 1.20
	sliceTopIdx := oldStart
	sinCheckCounter := 0

	for start-1 >= lowerBoundIdx {
		cur := rows[start]
		prv := rows[start-1]
		dt := cur.Time.Sub(prv.Time).Seconds()
		if dt <= 0 || dt > float64(cfg.SmartRecMaxGapS) {
			dprintf("BACK BREAK gap dt=%.1f at start=%d t=%s\n", dt, start, cur.Time.Format("15:04:05"))
			break
		}
		// effectiveSpeedCfg(rows, k) = haversine(rows[k-1]→rows[k]) / dt; that
		// is the speed entering 'cur' from 'prv', exactly what we want when
		// deciding whether to absorb prv into the ride.
		spd, spdOK := effectiveSpeedCfg(rows, start, cfg)
		isStop := spdOK && spd < cfg.MinSpeedMs
		if isStop {
			if stopStartIdx == -1 {
				stopStartIdx = start
				dprintf("BACK STOP enter at start=%d t=%s spd=%.2f\n", start, cur.Time.Format("15:04:05"), spd)
			}
			stopAccumS += dt
			if stopAccumS > backwardStopToleranceS {
				dprintf("BACK BREAK stop overflow %.1fs\n", stopAccumS)
				start = stopStartIdx
				break
			}
			start--
			continue
		}
		if spdOK && spd > overBandBreakSpdMs {
			dprintf("BACK BREAK over-band spd=%.1f at start=%d\n", spd, start)
			break
		}
		if stopStartIdx != -1 {
			stopStartIdx = -1
			stopAccumS = 0
		}
		// Altitude-reversal anchor.
		if (kind == "high" || kind == "medium") && prv.AltitudeM != nil && !math.IsNaN(initialAlt) {
			if goingUp {
				if *prv.AltitudeM > initialAlt+peakDropThreshM {
					dprintf("BACK BREAK alt-reversal prv=%.1f init=%.1f at start=%d\n", *prv.AltitudeM, initialAlt, start)
					break
				}
			} else {
				if *prv.AltitudeM < initialAlt-peakDropThreshM {
					dprintf("BACK BREAK alt-reversal(down) prv=%.1f init=%.1f at start=%d\n", *prv.AltitudeM, initialAlt, start)
					break
				}
			}
		}
		// Alt-advance stall (high/medium): walking backward, the ride direction
		// expects altitude to keep dropping (goingUp) / rising (descending).
		// When that advance stalls for altAdvanceStallBackS seconds, we have
		// crossed out of the lift line into the boarding queue — boarding-plateau case
		// "start before gondola" is exactly this shape: skiers shuffle 50–
		// 100 m on flat terrain before stepping onto the moving gondola, and
		// the per-step speed sits above MinSpeedMs so the speed-stop branch
		// never fires.
		if (kind == "high" || kind == "medium") && prv.AltitudeM != nil && !math.IsNaN(extremeAlt) {
			advanced := false
			if goingUp {
				if *prv.AltitudeM < extremeAlt-altAdvanceMinM {
					extremeAlt = *prv.AltitudeM
					lastAdvanceTime = prv.Time
					lastAdvanceIdx = start - 1
					advanced = true
				}
			} else {
				if *prv.AltitudeM > extremeAlt+altAdvanceMinM {
					extremeAlt = *prv.AltitudeM
					lastAdvanceTime = prv.Time
					lastAdvanceIdx = start - 1
					advanced = true
				}
			}
			if !advanced && lastAdvanceTime.Sub(prv.Time).Seconds() > altAdvanceStallBackS {
				dprintf("BACK BREAK alt-stall extreme=%.1f prv=%.1f stall=%.1fs at start=%d pin->lastAdvIdx=%d\n",
					extremeAlt, *prv.AltitudeM, lastAdvanceTime.Sub(prv.Time).Seconds(), start, lastAdvanceIdx)
				if lastAdvanceIdx > start && lastAdvanceIdx <= oldStart {
					start = lastAdvanceIdx
				}
				break
			}
		}
		sinCheckCounter++
		if sinCheckCounter >= backwardSinCheckEvery && (kind == "high" || kind == "medium") {
			if start-1 < sliceTopIdx {
				sin := windowSinuosity(rows, start-1, sliceTopIdx)
				if sin > backwardSinMiniMax {
					dprintf("BACK BREAK mini-sin=%.3f > %.3f for [%d..%d]\n",
						sin, backwardSinMiniMax, start-1, sliceTopIdx)
					break
				}
			}
			sliceTopIdx = start - 1
			sinCheckCounter = 0
		}
		start--
	}
	return start
}

// windowSinuosity returns cumulative-distance / straight-line-distance for
// rows[start:end+1]. Mirrors the sinuosity branch of computeStats but skips
// altitude / speed work — used by extendCandidate's bidir loop to break on
// non-lift terrain without paying the full computeStats cost on every probe.
func windowSinuosity(rows []StreamPoint, start, end int) float64 {
	if start >= end || start < 0 || end >= len(rows) {
		return 1.0
	}
	var firstLat, firstLng, lastLat, lastLng float64
	var haveFirst, haveLast bool
	var cumDist float64
	var prev *StreamPoint
	for k := start; k <= end; k++ {
		p := rows[k]
		if p.Lat == nil || p.Lng == nil {
			continue
		}
		if !haveFirst {
			firstLat, firstLng = *p.Lat, *p.Lng
			haveFirst = true
		}
		lastLat, lastLng = *p.Lat, *p.Lng
		haveLast = true
		if prev != nil {
			cumDist += Haversine(*prev.Lat, *prev.Lng, *p.Lat, *p.Lng)
		}
		pp := p
		prev = &pp
	}
	if !haveFirst || !haveLast {
		return 1.0
	}
	straight := Haversine(firstLat, firstLng, lastLat, lastLng)
	return cumDist / math.Max(straight, 1)
}

// trimTrailingChunks walks chunkedSpeeds([start,end], 10s) from the back and
// drops trailing chunks whose mean speed < 0.5 × median(chunk-speeds). Returns
// the index of the last sample inside the kept chunks. Conservative guards:
// only trims when at least 6 chunks remain (so the median is meaningful) and
// when the trim doesn't shrink below 6 chunks.
// trimTrailingLowSpeedSamples walks end backward sample-by-sample while the
// effective speed at end is below MinSpeedMs. Used as the alternative to
// trimTrailingChunks for fast-lift stop-indicator endings: the cable's decel
// zone (samples with speed ≥ MinSpeedMs but below cruise) stays in the ride,
// while the truly stationary tail (speed < MinSpeedMs at the bergstation
// arrival) gets peeled off. Conservative — never removes more than ~5 samples
// in practice because BREAK disembark fires on the FIRST low-speed sample.
func trimTrailingLowSpeedSamples(rows []StreamPoint, start, end int, cfg DetectorConfig) int {
	for end > start+1 {
		spd, ok := effectiveSpeedCfg(rows, end, cfg)
		if !ok || spd >= cfg.MinSpeedMs {
			return end
		}
		end--
	}
	return end
}

// trimTrailingAltPlateau walks end backward sample-by-sample while the
// trailing 5 s altitude rate at end is essentially flat (< 0.15 m/s)
// AND every sample in that window kept moving at cable-cruise speed.
// Used after trimTrailingLowSpeedSamples for fast-lift stop-indicator
// endings to catch walk-off zones where the rider walks 10–35 m past the
// cable terminal at 1–3 m/s.
//
// Two gates, both required:
//   1. alt-rate < 0.15 m/s — well below fast-lift cable cruise (0.30+) and
//      below the natural cable-decel slope (0.20–0.40 m/s on alpine ski-resort case-class
//      gondolas the trim must preserve).
//   2. min speed in window ≥ 3.0 m/s — fast-lift cable cruise sits at
//      3.5–9 m/s and stays steady through pylon transitions / mid-cable
//      horizontal sections (multi-section case 1 multi-A 08:50:49–08:51:00: alt-rate
//      0.05 m/s while every sample holds 4.85–4.95 m/s — that is mid-cable,
//      NOT walk-off). A bergstation walk-off always contains at least one
//      sub-3 m/s sample: pure walking (multi-resort case r9: min 0.69 m/s) or the
//      bügel-jump-and-slide pattern of T-bar lifts (T-bar drag-lift case Lift A:
//      min 2.45 m/s while the rider accelerates off the bar). Using the
//      window minimum (not mean) is what discriminates those cases — the
//      mean of the slide pattern climbs back above 3 m/s once the rider
//      gains glide momentum, but the entry samples never do.
// The fast-lift branch (rideAltRate ≥ 0.30) is the only caller, so the 3 m/s
// floor sits comfortably above the slowest infrastructure that hits this
// path; drag-lifts and slow chairs route through trimTrailingChunks instead.
//
// Bounded by MinDurationS so a heavily over-trimmed window does not
// invalidate the ride downstream.
func trimTrailingAltPlateau(rows []StreamPoint, start, end int) int {
	const windowS = 5.0
	const flatRateMs = 0.15
	const cableSpeedFloorMs = 3.0
	const minRideDurS = 60.0
	startT := rows[start].Time
	for end > start+5 {
		if rows[end].AltitudeM == nil {
			return end
		}
		cutoffT := rows[end].Time.Add(-time.Duration(windowS * float64(time.Second)))
		if cutoffT.Before(startT) {
			return end
		}
		ref := -1
		for k := end - 1; k > start; k-- {
			if !rows[k].Time.After(cutoffT) {
				ref = k
				break
			}
		}
		if ref < 0 || rows[ref].AltitudeM == nil {
			return end
		}
		dur := rows[end].Time.Sub(rows[ref].Time).Seconds()
		// Sampling-gap guard: if the 5 s window only covers a fraction of
		// the expected duration (e.g. smart-recording gap, FIT 1 Hz vs
		// stationary), the rate becomes noisy. Require at least 3.5 s.
		if dur < 3.5 {
			return end
		}
		rate := (*rows[end].AltitudeM - *rows[ref].AltitudeM) / dur
		if rate >= flatRateMs {
			return end
		}
		// Cable-cruise discriminator: if EVERY sample in the window held
		// cable-cruise speed, the alt-flat is a mid-cable horizontal section,
		// not a bergstation walk-off. The window is small (≈5 s, 5–6 samples
		// at 1 Hz). Nil speed samples are skipped (medium-confidence rides
		// without sensor speed); if the window has no usable speed at all,
		// fall through to the original alt-only behaviour.
		speedSeen := false
		speedMin := math.Inf(1)
		for k := ref; k <= end; k++ {
			if rows[k].SpeedMs != nil {
				speedSeen = true
				if *rows[k].SpeedMs < speedMin {
					speedMin = *rows[k].SpeedMs
				}
			}
		}
		if speedSeen && speedMin >= cableSpeedFloorMs {
			return end
		}
		// Stop when peeling another sample would shrink the ride below
		// MinDurationS. Cheaper than trial-trim-then-validateRide.
		if rows[end-1].Time.Sub(startT).Seconds() < minRideDurS {
			return end
		}
		end--
	}
	return end
}

func trimTrailingChunks(rows []StreamPoint, start, end int) int {
	const chunkSecs = 10.0
	chunks, _ := chunkedSpeeds(rows, start, end, chunkSecs)
	if len(chunks) < 6 {
		return end
	}
	med := median(chunks)
	if med <= 0 {
		return end
	}
	threshold := 0.5 * med
	keep := len(chunks)
	for keep > 0 && chunks[keep-1] < threshold {
		keep--
	}
	if keep == len(chunks) || keep < 6 {
		return end
	}
	cutoffTime := rows[start].Time.Add(time.Duration(keep*int(chunkSecs)) * time.Second)
	for k := end; k > start; k-- {
		if !rows[k].Time.After(cutoffTime) {
			return k
		}
	}
	return end
}

func shouldCloseOnResume(rows []StreamPoint, start, stopIdx, resumeIdx int, cfg DetectorConfig) bool {
	if rows[start].AltitudeM == nil || rows[stopIdx].AltitudeM == nil || rows[resumeIdx].AltitudeM == nil {
		return false
	}
	before := *rows[stopIdx].AltitudeM - *rows[start].AltitudeM
	after := *rows[resumeIdx].AltitudeM - *rows[stopIdx].AltitudeM
	return signWithThresh(before, 2.0)*signWithThresh(after, 2.0) < 0
}

func validateRide(ws stats, kind string, cfg DetectorConfig) bool {
	if ws.sinuosity > cfg.MaxSinuosity {
		return false
	}
	switch kind {
	case "high":
		if ws.durationS < float64(cfg.MinDurationS) {
			return false
		}
		if !ws.speedsOK || ws.meanSpeed < cfg.MinSpeedMs || ws.meanSpeed > cfg.MaxSpeedMs {
			return false
		}
		if ws.speedCV >= cfg.MaxSpeedCV {
			return false
		}
		// Minimum total ascent: real lifts cover at least 40 m of vertical
		// (the slowest T-bar lift in the corpus rises 110 m; the shortest
		// chairlift segment ~50 m). Anything below 40 m is a trail bump
		// that happens to pass the speed/sin gates.
		absAlt := math.Max(ws.altPosDelta, ws.altNegDelta)
		if absAlt < 40 {
			return false
		}
		absRate := math.Abs(ws.altRateMs)
		if absRate < 0.05 {
			return false
		}
		// Vertical-rate sanity: steep aerial tramways reach ~3.8 m/s
		// vertical (alpine bikepark mountain station case 2 Station Scharmoin: 906 m in
		// 240 s = 3.78 m/s). Cap at 5 m/s to keep instantaneous GPS /
		// altimeter spikes (typically 7+ m/s when smeared over a
		// 30-60 s window) from validating.
		if absRate > 5.0 {
			return false
		}
		gradient := absRate / math.Max(ws.meanSpeed, 0.001)
		if gradient <= cfg.MinGradient {
			return false
		}
		// Slow-band tightening: catch push/walking phases between proper
		// lifts. They share the slow-speed signature (1–2 m/s) with a real
		// lift but run on a flat hang (~5–7°, gradient < 0.15). FL-1
		// ride[03] (242 s/40 m, gradient 0.136) and bikepark transfer case-2 ride[01]
		// (368 s/52 m, gradient 0.116) are exactly this shape. The check
		// does not endanger genuine slow lifts: T-bar drag-lift case's T-bar lifts
		// run at the same 1.9–2.1 m/s but on a 15–18° hang
		// (gradient ≈ 0.30), well above the 0.15 floor.
		if ws.meanSpeed < 2.0 && gradient < 0.15 {
			return false
		}
		// Monotonicity check: real ascending lifts (T-bar, chairlift,
		// gondola, aerial tramway) have a minor descent component from
		// GPS jitter only — typically 0–5 % of ascent, never above ~6 %
		// across the validated corpus (cycling-computer device ride[5] L05 sits at 5.3 %,
		// the highest). A larger descent signals a hike/push/walk segment
		// that happened to clear the speed/sin gates. multi-section case 1 ride[6]
		// (asc 119 m, desc 21 m = 18 %, mean spd 1.1 m/s, sin 1.05) is the
		// calibrated FP. Threshold 10 % + 5 m absolute keeps a margin to
		// the L05-class real lifts. Bidir kind has its own paired
		// validation; descent is required there, not rejected.
		if ws.altNegDelta > 5.0 && ws.altNegDelta > 0.10*ws.altPosDelta {
			return false
		}
		return true
	case "bidir":
		if ws.durationS < float64(cfg.BidirMinDurationS) {
			return false
		}
		// True bidirectional cable cars (bidirectional aerial tramway-class) drop AND rise
		// within one ride. Reject "bidir" candidates that are pure
		// ascents (asc>0, desc≈0) or pure descents (asc≈0, desc>0).
		// The minimum-each-direction floor is asymmetric on purpose:
		// an aerial tramway passing through the dip carries the smaller
		// direction by 10-30 m, never less.
		minDir := math.Min(ws.altPosDelta, ws.altNegDelta)
		if minDir < 10.0 {
			return false
		}
		if ws.altPosDelta+ws.altNegDelta < cfg.BidirAltAbsM {
			return false
		}
		// Cumulative-vertical-rate cap: real aerial tramways run < 1.5 m/s
		// vertical accumulated. A bidir candidate with > 3 m/s
		// (altPos+altNeg)/dur is barometer-noise (multi-section case 1 14:07:21:
		// 354m up + 336m down in 145 s = 4.76 m/s — physically impossible
		// on a real lift). Mirrors the high-kind sanity cap on absRate.
		if (ws.altPosDelta+ws.altNegDelta)/math.Max(ws.durationS, 1) > 3.0 {
			return false
		}
		if ws.sinuosity > cfg.BidirSinuosityMax {
			return false
		}
		if !ws.speedsOK || ws.meanSpeed < cfg.MinSpeedMs || ws.meanSpeed > cfg.MaxSpeedMs {
			return false
		}
		// aerial tramway-class minimum mean speed: real bidirectional cable
		// cars (bidirectional aerial tramway-class) run at 6–10 m/s. A bidir candidate with
		// mean < 4 m/s is not an aerial tramway — it is most likely a slow
		// MTB segment crossing a saddle (multi-section case 1 13:50:49: mean
		// 1.05 m/s, 268 m absolute alt change in 170 s, sin 1.05 — the
		// shape of a bidir lift but at walking pace, physically only
		// possible as an ascent-rich trail not a cable car).
		if ws.meanSpeed < 4.0 {
			return false
		}
		if ws.speedCV >= cfg.MaxSpeedCV {
			return false
		}
		return true
	case "medium":
		if ws.durationS < float64(cfg.NoSpeedMinDurationS) {
			return false
		}
		if math.Abs(ws.altRateMs) <= cfg.NoSpeedMinAltRateMs {
			return false
		}
		return true
	}
	return false
}

// altSecondsAgo searches backward from idx for the most recent sample whose
// timestamp is at least `secs` seconds older than rows[idx].Time. It does not
// look past `start`. Returns altitude and ok=true on hit.
func altSecondsAgo(rows []StreamPoint, idx int, secs float64, start int) (float64, bool) {
	if idx <= start || idx >= len(rows) {
		return 0, false
	}
	target := rows[idx].Time.Add(-time.Duration(secs * float64(time.Second)))
	for k := idx - 1; k >= start; k-- {
		if !rows[k].Time.After(target) {
			if rows[k].AltitudeM == nil {
				continue
			}
			return *rows[k].AltitudeM, true
		}
	}
	return 0, false
}

func boundarySpeedDropOK(rows []StreamPoint, start, end int, cfg DetectorConfig) bool {
	dropFound := false
	if meanSpeedAround(rows, start, -cfg.BoundarySpeedDropWindowS, cfg) < cfg.BoundarySpeedDropMs {
		dropFound = true
	}
	if !dropFound && meanSpeedAround(rows, end, +cfg.BoundarySpeedDropWindowS, cfg) < cfg.BoundarySpeedDropMs {
		dropFound = true
	}
	return dropFound
}

// meanSpeedAround returns the mean effective speed in a window of N seconds
// before (windowS<0) or after (windowS>0) anchor index. Returns +Inf if no
// usable samples — interpreted as "no drop observed" by the caller.
func meanSpeedAround(rows []StreamPoint, anchor, windowS int, cfg DetectorConfig) float64 {
	if anchor < 0 || anchor >= len(rows) {
		return math.Inf(1)
	}
	t0 := rows[anchor].Time
	var sum float64
	var n int
	if windowS < 0 {
		for k := anchor - 1; k >= 0; k-- {
			if t0.Sub(rows[k].Time).Seconds() > float64(-windowS) {
				break
			}
			if v, ok := effectiveSpeedCfg(rows, k, cfg); ok {
				sum += v
				n++
			}
		}
	} else {
		for k := anchor + 1; k < len(rows); k++ {
			if rows[k].Time.Sub(t0).Seconds() > float64(windowS) {
				break
			}
			if v, ok := effectiveSpeedCfg(rows, k, cfg); ok {
				sum += v
				n++
			}
		}
	}
	if n == 0 {
		return math.Inf(1)
	}
	return sum / float64(n)
}

// helpers ────────────────────────────────────────────────────────────────────

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func coeffOfVariation(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := mean(xs)
	if m == 0 {
		return 0
	}
	var sq float64
	for _, x := range xs {
		d := x - m
		sq += d * d
	}
	std := math.Sqrt(sq / float64(len(xs)))
	return std / math.Max(m, 0.001)
}

func signWithThresh(x, thresh float64) float64 {
	if x > thresh {
		return 1
	}
	if x < -thresh {
		return -1
	}
	return 0
}

