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
func DetectHeuristic(rows []StreamPoint, cfg DetectorConfig) []LiftRide {
	if len(rows) < cfg.MinSamples {
		return nil
	}
	var rides []LiftRide
	i := 0
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
		rides = append(rides, LiftRide{
			StartedAt:  rows[start].Time,
			EndedAt:    rows[finalEnd].Time,
			DurationS:  int(rows[finalEnd].Time.Sub(rows[start].Time).Seconds()),
			AscentM:    final.altPosDelta,
			DescentM:   final.altNegDelta,
			DistanceM:  final.cumDist,
			Confidence: kind,
		})
		i = finalEnd + 1
	}
	return rides
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
		// over median) instead of stddev/mean CV. Lift edges (boarding
		// rampup, disembark rampdown) push 5-10 chunks per side off the
		// stable core; mean-based CV would let those outliers dominate
		// even on a perfectly steady gondola. MAD/median is unaffected
		// by < 50% outliers.
		s.speedCV = robustDispersion(chunked)
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
		if ws.speedCV >= cfg.MaxSpeedCV {
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
	const peakDropThreshM = 5.0
	const peakAdvanceMinM = 3.0 // robust to ±1 m barometer noise
	const stallTimeoutS = 20.0
	const stallMinDropM = 3.0 // require ≥3m drop off peak (1m would trigger on barometer noise / mid-lift micro-plateaus that the 60s plateau check already catches)
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

	for end+1 < len(rows) {
		cur := rows[end]
		nxt := rows[end+1]
		dt := nxt.Time.Sub(cur.Time).Seconds()
		if dt <= 0 || dt > float64(cfg.SmartRecMaxGapS) {
			dprintf("BREAK gap dt=%.1f at end=%d t=%s\n", dt, end, cur.Time.Format("15:04:05"))
			break
		}
		spd, spdOK := effectiveSpeedCfg(rows, end+1, cfg)
		// Stop branch
		isStop := spdOK && spd < cfg.MinSpeedMs
		if isStop {
			if stopStartIdx == -1 {
				stopStartIdx = end
				dprintf("STOP enter at end=%d t=%s spd=%.2f\n", end, cur.Time.Format("15:04:05"), spd)
			}
			stopAccumS += dt
			if stopAccumS > float64(cfg.StopToleranceS) {
				dprintf("BREAK stop overflow %.1fs\n", stopAccumS)
				end = stopStartIdx
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
				break
			}
			dprintf("RESUME ok stopAccum=%.1fs stopStart=%d\n", stopAccumS, stopStartIdx)
			stopStartIdx = -1
			stopAccumS = 0
		}
		// Out-of-band speed → close (skiing speed > 12, stationary already handled)
		if spdOK && (spd > cfg.MaxSpeedMs) {
			dprintf("BREAK over-band spd=%.1f at end=%d t=%s\n", spd, end, cur.Time.Format("15:04:05"))
			break
		}
		// 60-second flat-plateau check: if altitude has barely changed
		// over the past 60 s the lift is stationary or the rider has
		// reached the top — close the window. This is a backstop that
		// catches mid-tour station-to-station transfers (e.g. transfer case
		// transfer-station case → secondary transfer station at 1334 m for 4 minutes) where
		// altitude stays exactly flat so the drop-based stall does not
		// fire, AND boarding-zone idle time longer than 60 s.
		if (kind == "high" || kind == "medium") && nxt.AltitudeM != nil {
			const plateauWinS = 60.0
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
	if (kind == "high" || kind == "medium") && peakAdvanceIdx > start && peakAdvanceIdx < end {
		return peakAdvanceIdx
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
		// (the slowest Schlepplift in the corpus rises 110 m; the shortest
		// Sessellift segment ~50 m). Anything below 40 m is a trail bump
		// that happens to pass the speed/sin gates.
		absAlt := math.Max(ws.altPosDelta, ws.altNegDelta)
		if absAlt < 40 {
			return false
		}
		absRate := math.Abs(ws.altRateMs)
		if absRate < 0.05 {
			return false
		}
		// Vertical-rate sanity: cable cars top out around 2.6 m/s
		// vertical (steep modern gondolas like Crap Sogn Gion). Any
		// candidate ride showing > 3 m/s vertical is a GPS/altimeter
		// glitch — usually a sample-jump in the underlying stream.
		if absRate > 3.0 {
			return false
		}
		gradient := absRate / math.Max(ws.meanSpeed, 0.001)
		if gradient <= cfg.MinGradient {
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
		// a Pendelbahn passing through the dip carries the smaller
		// direction by 10-30 m, never less.
		minDir := math.Min(ws.altPosDelta, ws.altNegDelta)
		if minDir < 10.0 {
			return false
		}
		if ws.altPosDelta+ws.altNegDelta < cfg.BidirAltAbsM {
			return false
		}
		if ws.sinuosity > cfg.BidirSinuosityMax {
			return false
		}
		if !ws.speedsOK || ws.meanSpeed < cfg.MinSpeedMs || ws.meanSpeed > cfg.MaxSpeedMs {
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

