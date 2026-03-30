package pmc

import (
	"math"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// --- Normalized Power (NP) ---

// CalcNP calculates Normalized Power from a 1 Hz power stream.
// Returns 0 if stream has fewer than 30 samples.
// Zeros are included so coasting contributes to the rolling average.
func CalcNP(power []float64) float64 {
	if len(power) < 30 {
		return 0
	}
	power = repairDropouts(power)
	return calcNormalized(power)
}

// calcNormalized applies 30s SMA → 4th power → mean → 4th root.
// Used by both CalcNP (power) and CalcNGP (speed). Does NOT call repairDropouts.
func calcNormalized(stream []float64) float64 {
	if len(stream) < 30 {
		return 0
	}
	const window = 30
	sma := make([]float64, len(stream)-window+1)
	var windowSum float64
	for i := range window {
		windowSum += stream[i]
	}
	sma[0] = windowSum / float64(window)
	for i := 1; i < len(sma); i++ {
		windowSum += stream[i+window-1] - stream[i-1]
		sma[i] = windowSum / float64(window)
	}
	var sum4 float64
	for _, v := range sma {
		v2 := v * v
		sum4 += v2 * v2
	}
	return math.Pow(sum4/float64(len(sma)), 0.25)
}

// repairDropouts interpolates short power dropouts (1-5 samples of 0W
// surrounded by >50W values). Gaps >5 samples are NOT interpolated.
func repairDropouts(power []float64) []float64 {
	result := make([]float64, len(power))
	copy(result, power)
	for i := 1; i < len(result)-1; i++ {
		if result[i] != 0 {
			continue
		}
		if result[i-1] <= 50 {
			continue
		}
		gapEnd := i
		for gapEnd < len(result) && result[gapEnd] == 0 {
			gapEnd++
		}
		gapLen := gapEnd - i
		if gapLen > 5 || gapEnd >= len(result) || result[gapEnd] <= 50 {
			i = gapEnd
			continue
		}
		startVal := result[i-1]
		endVal := result[gapEnd]
		for j := range gapLen {
			frac := float64(j+1) / float64(gapLen+1)
			result[i+j] = startVal + (endVal-startVal)*frac
		}
		i = gapEnd
	}
	return result
}

// --- Coggan hrTSS ---

// CalcHrTSS computes second-by-second Coggan hrTSS from an HR stream.
// Returns 0 if LTHR <= RestingHR or stream is empty.
// Constants 0.64 and 1.92 are Coggan's fit for the HR-to-VO2/lactate curve.
// 1 hour at exactly LTHR = 100 hrTSS (by construction).
func CalcHrTSS(hrStream []int, lthr, restingHR, maxHR int) float64 {
	if lthr <= restingHR || len(hrStream) == 0 {
		return 0
	}
	lthrRange := float64(lthr - restingHR)
	normFactor := 0.64 * math.Exp(1.92) // intensity at HRr=1.0

	maxValidHR := maxHR + 10 // artifact detection threshold

	var totalIntensity float64
	for _, hr := range hrStream {
		if hr <= 0 || hr > maxValidHR {
			continue
		}
		hrr := float64(hr-restingHR) / lthrRange
		if hrr < 0 {
			hrr = 0
		}
		totalIntensity += hrr * 0.64 * math.Exp(1.92*hrr)
	}
	return totalIntensity / (3600 * normFactor) * 100
}

// CalcHrTSSAvg computes Coggan hrTSS from average HR (fallback when no stream).
// Less accurate than second-by-second: underestimates intervals by 10-30%.
func CalcHrTSSAvg(avgHR float64, lthr, restingHR, durationS int) float64 {
	if lthr <= restingHR || durationS <= 0 || avgHR <= 0 {
		return 0
	}
	lthrRange := float64(lthr - restingHR)
	normFactor := 0.64 * math.Exp(1.92)

	hrr := (avgHR - float64(restingHR)) / lthrRange
	if hrr < 0 {
		hrr = 0
	}
	intensity := hrr * 0.64 * math.Exp(1.92*hrr)
	return float64(durationS) / 3600 * (intensity / normFactor) * 100
}

// --- Edwards TRIMP ---

// CalcEdwardsTRIMP computes Edwards TRIMP from an HR stream.
// Uses standard factors: Zone 1-5 = 1,2,3,4,5.
// Unit: TRIMP-minutes (1 minute in Zone 5 = 5.0 TRIMP-minutes).
func CalcEdwardsTRIMP(hrStream []int, restingHR, maxHR int) float64 {
	hrRange := float64(maxHR - restingHR)
	if hrRange <= 0 || len(hrStream) == 0 {
		return 0
	}
	type zone struct{ min, max, factor float64 }
	zones := []zone{
		{0.50, 0.60, 1},
		{0.60, 0.70, 2},
		{0.70, 0.80, 3},
		{0.80, 0.90, 4},
		{0.90, 1.01, 5},
	}
	var trimp float64
	for _, hr := range hrStream {
		if hr <= 0 {
			continue
		}
		ratio := (float64(hr) - float64(restingHR)) / hrRange
		for _, z := range zones {
			if ratio >= z.min && ratio < z.max {
				trimp += z.factor / 60.0
				break
			}
		}
	}
	return trimp
}

// --- rTSS (Running, NGP) ---

// minettiCr returns the energy cost coefficient (J/kg/m) for a given grade.
// Grade is a decimal (-0.10 = -10%, 0.10 = +10%). Clamped to ±45%.
// Minetti et al. 2002, J Appl Physiol 93:1039-1046.
// Legacy: kept as reference, superseded by re3MetabolicRate.
func minettiCr(grade float64) float64 {
	if grade < -0.45 {
		grade = -0.45
	}
	if grade > 0.45 {
		grade = 0.45
	}
	i := grade
	return 155.4*math.Pow(i, 5) - 30.4*math.Pow(i, 4) - 43.3*math.Pow(i, 3) +
		46.3*math.Pow(i, 2) + 19.5*i + 3.6
}

// re3MetabolicRate returns metabolic energy expenditure rate (W/kg)
// for running at the given speed (m/s) and grade (decimal rise/run).
// Looney, Hoogkamer, Kram (2025), Eq. 4.
// Valid: speed 1.78-5.70 m/s, grade -0.45 to +0.82.
func re3MetabolicRate(speed, grade float64) float64 {
	if grade < -0.45 {
		grade = -0.45
	}
	if grade > 0.82 {
		grade = 0.82
	}
	if speed < 1.0 {
		speed = 1.0
	}
	// RE3 Level (Eq. 1)
	mLevel := 4.43 + 1.51*speed + 0.37*speed*speed
	// RE3 Graded term (Eq. 4): β₁=30.43, β₂=1.133, β₃=1.056, β₄=43
	exp1 := math.Pow(1.056, 100.0*grade+43.0)
	exp2 := math.Pow(1.133, 1.0-exp1)
	gradeTerm := 30.43 * speed * grade * (1.0 - exp2)
	result := mLevel + gradeTerm
	// Floor: RE3 polynomial can yield < 1 W/kg below the validated speed range
	// (1.78 m/s). 1.0 W/kg is below resting metabolic rate for any locomotion.
	if result < 1.0 {
		return 1.0
	}
	return result
}

// re3InvertLevel solves RE3 level equation for speed given metabolic rate.
// Ṁ = 4.43 + 1.51*S + 0.37*S² → quadratic formula.
func re3InvertLevel(mdot float64) float64 {
	const a, b, c0 = 0.37, 1.51, 4.43
	disc := b*b - 4*a*(c0-mdot)
	// Unreachable in normal operation: disc < 0 only when mdot < ~2.89 W/kg,
	// which is below re3MetabolicRate's floor of 1.0.
	if disc < 0 {
		return 0
	}
	s := (-b + math.Sqrt(disc)) / (2 * a)
	if s < 0 {
		return 0
	}
	return s
}

// re3GradeAdjustedSpeed converts actual speed at a grade to equivalent flat speed.
// Uses exact quadratic inversion of RE3 level equation.
func re3GradeAdjustedSpeed(speed, grade float64) float64 {
	if speed <= 0 {
		return 0
	}
	mdotGraded := re3MetabolicRate(speed, grade)
	return re3InvertLevel(mdotGraded)
}

// CalcNGP calculates Normalized Graded Pace from speed and grade streams.
// If gapStream (Garmin GAP field 140) is provided, it's used directly.
// Unlike CalcNP, does NOT apply repairDropouts (speed=0 = standing, not dropout).
func CalcNGP(speeds, grades []float64, gapStream []float64) float64 {
	n := len(speeds)
	if n < 30 {
		return 0
	}
	gasStream := make([]float64, n)
	for i := range n {
		if gapStream != nil && i < len(gapStream) && gapStream[i] > 0 {
			gasStream[i] = gapStream[i] // Garmin native GAP preferred
		} else if i < len(grades) {
			gasStream[i] = re3GradeAdjustedSpeed(speeds[i], grades[i])
		} else {
			gasStream[i] = speeds[i]
		}
	}
	// NP algorithm without repairDropouts
	return calcNormalized(gasStream)
}

// --- sTSS (Swimming) ---

// CalcSwimTSS computes swimming TSS from distance, moving time, and CSS.
// No pace normalization — uses average moving speed per the established rTSS convention.
func CalcSwimTSS(distanceM float64, movingTimeS int, cssMps float64) float64 {
	if movingTimeS <= 0 || cssMps <= 0 || distanceM <= 0 {
		return 0
	}
	avgSpeed := distanceM / float64(movingTimeS)
	iF := avgSpeed / cssMps
	return float64(movingTimeS) / 3600 * iF * iF * 100
}

// --- Stream Extractors ---

// DeduplicateStreamsByTimestamp merges multi-row-per-timestamp records into
// one StreamPoint per unique timestamp, taking the first non-nil value for
// each field. Devices like Apple Watch, COROS, and Wahoo write 2-7 record
// messages per second with different field sets (e.g. one row has power,
// another has speed). Without deduplication, nil power rows become 0W and
// inflate the zero count, skewing NP by ~20%.
func DeduplicateStreamsByTimestamp(points []model.StreamPoint) []model.StreamPoint {
	if len(points) == 0 {
		return nil
	}
	result := make([]model.StreamPoint, 0, len(points))
	current := points[0]

	for i := 1; i < len(points); i++ {
		if !points[i].Time.Equal(current.Time) {
			result = append(result, current)
			current = points[i]
			continue
		}
		// Same timestamp: merge non-nil fields into current
		if current.PowerW == nil && points[i].PowerW != nil {
			current.PowerW = points[i].PowerW
		}
		if current.HeartRate == nil && points[i].HeartRate != nil {
			current.HeartRate = points[i].HeartRate
		}
		if current.Cadence == nil && points[i].Cadence != nil {
			current.Cadence = points[i].Cadence
		}
		if current.SpeedMS == nil && points[i].SpeedMS != nil {
			current.SpeedMS = points[i].SpeedMS
		}
	}
	result = append(result, current)
	return result
}

// isFootSport returns true for sports where Stryd foot-pod power is more
// accurate than wrist-based estimates (running, hiking, walking).
func isFootSport(sport int16) bool {
	return sport == 1 || sport == 11 || sport == 17 || sport == 37
}

// ExtractPowerStream extracts power values from StreamPoints as float64 slice.
// Expects deduplicated input (one row per timestamp — see DeduplicateStreamsByTimestamp).
// nil PowerW → 0 so coasting contributes to the rolling average.
// For foot sports (running, hiking, walking), prefers the "Power" developer field
// (e.g. Stryd foot-pod) over the native PowerW field.
func ExtractPowerStream(points []model.StreamPoint, sport int16) []float64 {
	result := make([]float64, len(points))
	preferDevField := isFootSport(sport)
	for i, p := range points {
		if preferDevField && p.DeveloperFields != nil {
			if v, ok := p.DeveloperFields["Power"]; ok {
				switch val := v.(type) {
				case float64:
					result[i] = val
				case float32:
					result[i] = float64(val)
				}
				continue
			}
		}
		if p.PowerW != nil {
			result[i] = float64(*p.PowerW)
		} else if p.DeveloperFields != nil {
			if v, ok := p.DeveloperFields["Power"]; ok {
				switch val := v.(type) {
				case float64:
					result[i] = val
				case float32:
					result[i] = float64(val)
				}
			}
		}
	}
	return result
}

// ExtractHRStream extracts heart rate values as int slice.
func ExtractHRStream(points []model.StreamPoint) []int {
	result := make([]int, len(points))
	for i, p := range points {
		if p.HeartRate != nil {
			result[i] = int(*p.HeartRate)
		}
	}
	return result
}

// ExtractSpeedStream extracts speed values (m/s) as float64 slice.
func ExtractSpeedStream(points []model.StreamPoint) []float64 {
	result := make([]float64, len(points))
	for i, p := range points {
		if p.SpeedMS != nil {
			result[i] = *p.SpeedMS
		}
	}
	return result
}

// ExtractGradeStream extracts grade as decimal (0.10 = 10%).
// GradePct in the model is percentage — we divide by 100.
func ExtractGradeStream(points []model.StreamPoint) []float64 {
	result := make([]float64, len(points))
	for i, p := range points {
		if p.GradePct != nil {
			result[i] = *p.GradePct / 100.0
		}
	}
	return result
}

// ExtractGAPStream extracts Garmin GAP (field 140) as m/s.
// Returns nil if no GAP data found.
func ExtractGAPStream(points []model.StreamPoint) []float64 {
	result := make([]float64, len(points))
	hasAny := false
	for i, p := range points {
		if p.GapSpeedMS != nil && *p.GapSpeedMS > 0 {
			result[i] = *p.GapSpeedMS
			hasAny = true
		}
	}
	if !hasAny {
		return nil
	}
	return result
}

// --- TSS Pipeline Orchestrator ---

// TSSResult contains all computed metrics for a single activity.
type TSSResult struct {
	ComputedTSS    *float64
	ComputedNPW    *float64
	ComputedNGPMps *float64
	ComputedIF     *float64
	TSSMethod      string
	TRIMP          *float64
}

// ComputeTSS runs the full TSS computation pipeline for one activity.
// Method priority: power_np > power_avg > pace_ngp > pace_swim > hr_stream > hr_avg > none
// Edwards TRIMP is always computed in parallel if HR stream is available.
func ComputeTSS(
	sport, subSport int16,
	movingTimeS int,
	avgPowerW *float64,
	avgHR *float64,
	totalDistanceM *float64,
	powerStream []float64,
	hrStream []int,
	speedStream []float64,
	gradeStream []float64,
	gapStream []float64,
	th *Thresholds,
) *TSSResult {
	r := &TSSResult{TSSMethod: "none"}

	// Edwards TRIMP (always, if HR stream available)
	if len(hrStream) > 0 && th.MaxHR > th.RestingHR {
		trimp := CalcEdwardsTRIMP(hrStream, th.RestingHR, th.MaxHR)
		if trimp > 0 {
			r.TRIMP = &trimp
		}
	}

	// Method 1a: Power-NP TSS (power stream + FTP)
	if len(powerStream) >= 30 && th.FTP > 0 {
		np := CalcNP(powerStream)
		if np > 0 {
			iF := np / float64(th.FTP)
			tss := float64(movingTimeS) / 3600 * iF * iF * 100
			r.ComputedTSS = &tss
			r.ComputedNPW = &np
			r.ComputedIF = &iF
			r.TSSMethod = "power_np"
			return r
		}
	}

	// Method 1b: Power-avg TSS (avg power only, no stream)
	if avgPowerW != nil && *avgPowerW > 0 && th.FTP > 0 && len(powerStream) < 30 {
		iF := *avgPowerW / float64(th.FTP)
		tss := float64(movingTimeS) / 3600 * iF * iF * 100
		r.ComputedTSS = &tss
		r.ComputedIF = &iF
		r.TSSMethod = "power_avg"
		return r
	}

	// Method 2: rTSS (running NGP) — sport=1 (Running)
	if sport == 1 && len(speedStream) >= 30 && th.SpeedThreshold != nil && *th.SpeedThreshold > 0 {
		ngp := CalcNGP(speedStream, gradeStream, gapStream)
		if ngp > 0 {
			iF := ngp / *th.SpeedThreshold
			tss := float64(movingTimeS) / 3600 * iF * iF * 100
			r.ComputedTSS = &tss
			r.ComputedNGPMps = &ngp
			r.ComputedIF = &iF
			r.TSSMethod = "pace_ngp"
			return r
		}
	}

	// Method 3: sTSS (swimming) — sport=5 (Swimming)
	if sport == 5 && totalDistanceM != nil && *totalDistanceM > 0 && th.SpeedThreshold != nil && *th.SpeedThreshold > 0 {
		tss := CalcSwimTSS(*totalDistanceM, movingTimeS, *th.SpeedThreshold)
		if tss > 0 {
			avgSpeed := *totalDistanceM / float64(movingTimeS)
			iF := avgSpeed / *th.SpeedThreshold
			r.ComputedTSS = &tss
			r.ComputedIF = &iF
			r.TSSMethod = "pace_swim"
			return r
		}
	}

	// Method 4: hrTSS (second-by-second)
	if len(hrStream) > 0 && th.LTHR > th.RestingHR {
		tss := CalcHrTSS(hrStream, th.LTHR, th.RestingHR, th.MaxHR)
		if tss > 0 {
			r.ComputedTSS = &tss
			r.TSSMethod = "hr_stream"
			return r
		}
	}

	// Method 5: hrTSS (average fallback)
	if avgHR != nil && *avgHR > 0 && th.LTHR > th.RestingHR {
		tss := CalcHrTSSAvg(*avgHR, th.LTHR, th.RestingHR, movingTimeS)
		if tss > 0 {
			r.ComputedTSS = &tss
			r.TSSMethod = "hr_avg"
			return r
		}
	}

	return r
}
