package pmc

import (
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

const (
	ctlDays = 42.0
	atlDays = 7.0
)

var (
	ctlDecay  = math.Exp(-1 / ctlDays)
	ctlFactor = 1 - ctlDecay
	atlDecay  = math.Exp(-1 / atlDays)
	atlFactor = 1 - atlDecay
)

// EstimateTSS estimates Training Stress Score from activity data.
// Power-based TSS is preferred; falls back to HR-based estimation.
func EstimateTSS(avgPowerW *float64, ftpW float64, avgHR *float64, maxHR float64, movingTimeS int) float64 {
	if avgPowerW != nil && *avgPowerW > 0 && ftpW > 0 {
		intensityFactor := *avgPowerW / ftpW
		return (float64(movingTimeS) * *avgPowerW * intensityFactor) / (ftpW * 3600) * 100
	}

	if avgHR != nil && *avgHR > 0 && maxHR > 0 {
		return float64(movingTimeS) / 3600 * (*avgHR / maxHR) * 100
	}

	return 0
}

// CalculateTRIMP computes the Edwards TRIMP score from a heart rate stream.
// Each value in hrStream represents one second of data.
// Returns TRIMP in "TRIMP-minutes" (each zone-second contributes zone-factor/60).
func CalculateTRIMP(hrStream []int, restingHR, maxHR int) float64 {
	if len(hrStream) == 0 || maxHR <= restingHR {
		return 0
	}
	zones := []struct{ min, max, factor float64 }{
		{0.50, 0.60, 1},
		{0.60, 0.70, 2},
		{0.70, 0.80, 3},
		{0.80, 0.90, 5},
		{0.90, 1.01, 10},
	}
	hrRange := float64(maxHR - restingHR)
	var trimp float64
	for _, hr := range hrStream {
		if hr <= 0 {
			continue
		}
		ratio := (float64(hr) - float64(restingHR)) / hrRange
		if ratio <= 0 {
			continue
		}
		for _, z := range zones {
			if ratio >= z.min && ratio < z.max {
				trimp += z.factor / 60.0
				break
			}
		}
	}
	return trimp
}

// Calculate computes the PMC (CTL/ATL/TSB) series from a TSS-per-day map.
// dailyTSS maps date strings (YYYY-MM-DD) to TSS values.
// previousCTL and previousATL are the values from the day before `from`.
func Calculate(from, to time.Time, dailyTSS map[string]float64, previousCTL, previousATL float64) []model.PMCDay {
	days := int(to.Sub(from).Hours()/24) + 1
	if days <= 0 {
		return nil
	}

	result := make([]model.PMCDay, 0, days)
	ctl := previousCTL
	atl := previousATL

	for i := range days {
		d := from.AddDate(0, 0, i)
		dateStr := d.Format("2006-01-02")

		tss := dailyTSS[dateStr]

		// TSB uses yesterday's CTL and ATL
		tsb := ctl - atl

		// Exponential weighted moving average
		ctl = ctl*ctlDecay + tss*ctlFactor
		atl = atl*atlDecay + tss*atlFactor

		var acwr float64
		if ctl > 0 {
			acwr = math.Round(atl/ctl*100) / 100
		}

		result = append(result, model.PMCDay{
			Date: dateStr,
			TSS:  math.Round(tss*100) / 100,
			CTL:  math.Round(ctl*100) / 100,
			ATL:  math.Round(atl*100) / 100,
			TSB:  math.Round(tsb*100) / 100,
			ACWR: acwr,
		})
	}

	return result
}
