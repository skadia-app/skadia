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
	for i := 0; i < window; i++ {
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
		for j := 0; j < gapLen; j++ {
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

// CalcNGP calculates Normalized Graded Pace from speed and grade streams.
// If gapStream (Garmin GAP field 140) is provided, it's used directly.
// Unlike CalcNP, does NOT apply repairDropouts (speed=0 = standing, not dropout).
func CalcNGP(speeds, grades []float64, gapStream []float64) float64 {
	n := len(speeds)
	if n < 30 {
		return 0
	}
	crFlat := minettiCr(0) // 3.6
	gasStream := make([]float64, n)
	for i := 0; i < n; i++ {
		if gapStream != nil && i < len(gapStream) && gapStream[i] > 0 {
			gasStream[i] = gapStream[i]
		} else if i < len(grades) {
			cr := minettiCr(grades[i])
			gas := speeds[i] * cr / crFlat
			if gas < 0 {
				gas = 0
			}
			gasStream[i] = gas
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

// ExtractPowerStream extracts power values from StreamPoints as float64 slice.
// Nil values become 0 (coasting). *int16 → float64 conversion.
func ExtractPowerStream(points []model.StreamPoint) []float64 {
	result := make([]float64, len(points))
	for i, p := range points {
		if p.PowerW != nil {
			result[i] = float64(*p.PowerW)
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
