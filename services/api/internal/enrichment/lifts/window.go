package lifts

// buildInitialWindow advances j from i forward until either WindowDurationS
// seconds have elapsed AND at least MinSamples points were collected, OR the
// stream ends. It rejects the window early when a gap > SmartRecMaxGapS is
// encountered (the recording was paused — Mid-lift-resume must restart the
// sliding loop, not silently bridge the gap).
//
// Returns indices [start, end] inclusive and ok=true on success.
func buildInitialWindow(rows []StreamPoint, i int, cfg DetectorConfig) (start, end int, ok bool) {
	if i >= len(rows) {
		return 0, 0, false
	}
	start = i
	end = i
	for j := i + 1; j < len(rows); j++ {
		dt := rows[j].Time.Sub(rows[j-1].Time).Seconds()
		if dt > float64(cfg.SmartRecMaxGapS) {
			return 0, 0, false
		}
		end = j
		elapsed := rows[j].Time.Sub(rows[i].Time).Seconds()
		samples := j - i + 1
		if elapsed >= float64(cfg.WindowDurationS) && samples >= cfg.MinSamples {
			return start, end, true
		}
	}
	// Stream ended before WindowDurationS — accept iff sample count met.
	if end-start+1 >= cfg.MinSamples {
		elapsed := rows[end].Time.Sub(rows[start].Time).Seconds()
		if elapsed >= float64(cfg.WindowDurationS) {
			return start, end, true
		}
	}
	return 0, 0, false
}

// effectiveSpeed returns one speed value for stream point at index k, with
// preference: SpeedMs field if present and >= GPSDeltaSpeedFloorMs (sensor
// reports motion) → otherwise haversine(prev, cur) / dt → otherwise -1.
//
// The "0 wins over GPS" failure mode in the Python heuristic
// (services/ml/enrichment/lifts.py:306-313) is what the floor protects
// against: a single SpeedMs=0 in an otherwise GPS-moving window collapses
// CV to >> MaxSpeedCV and rejects the candidate even though the GPS
// clearly shows a lift.
func effectiveSpeed(rows []StreamPoint, k int) (float64, bool) {
	return effectiveSpeedCfg(rows, k, DefaultDetectorConfig(""))
}

func effectiveSpeedCfg(rows []StreamPoint, k int, cfg DetectorConfig) (float64, bool) {
	if k < 0 || k >= len(rows) {
		return 0, false
	}
	p := rows[k]
	if p.SpeedMs != nil && *p.SpeedMs >= cfg.GPSDeltaSpeedFloorMs {
		return *p.SpeedMs, true
	}
	if k == 0 {
		return 0, false
	}
	prev := rows[k-1]
	if p.Lat == nil || p.Lng == nil || prev.Lat == nil || prev.Lng == nil {
		return 0, false
	}
	dt := p.Time.Sub(prev.Time).Seconds()
	if dt <= 0 {
		return 0, false
	}
	// A recording-gap (Garmin Auto-Pause / Smart-Recording) makes
	// haversine/dt meaningless: the rider was usually stationary or in a
	// cabin and the GPS-delta blends "pre-pause stop" with "post-pause
	// motion" into a single fictitious low-speed reading. Skip it.
	if dt > float64(cfg.SmartRecMaxGapS) {
		return 0, false
	}
	d := Haversine(*prev.Lat, *prev.Lng, *p.Lat, *p.Lng)
	return d / dt, true
}

// effectiveSpeeds returns one speed per index in [start, end] following the
// preference order documented on effectiveSpeedCfg.
func effectiveSpeeds(rows []StreamPoint, start, end int, cfg DetectorConfig) []float64 {
	out := make([]float64, 0, end-start+1)
	for k := start; k <= end; k++ {
		if v, ok := effectiveSpeedCfg(rows, k, cfg); ok {
			out = append(out, v)
		}
	}
	return out
}
