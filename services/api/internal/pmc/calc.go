package pmc

import (
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// CalcACWR_RA computes Rolling Average ACWR (7:28, uncoupled).
// Acute = days 0-6 (current week), Chronic = days 7-34 (4 weeks BEFORE current week).
// Returns 0 if insufficient training history (< 7 training days in chronic window).
func CalcACWR_RA(dailyTSS map[string]float64, today time.Time) float64 {
	var acuteSum, chronicSum float64
	var chronicDaysWithTraining int
	for d := range 7 {
		key := today.AddDate(0, 0, -d).Format("2006-01-02")
		acuteSum += dailyTSS[key]
	}
	for d := 7; d < 35; d++ {
		key := today.AddDate(0, 0, -d).Format("2006-01-02")
		tss := dailyTSS[key]
		chronicSum += tss
		if tss > 0 {
			chronicDaysWithTraining++
		}
	}
	// Minimum: 7 training days in chronic 28-day window
	if chronicDaysWithTraining < 7 {
		return 0
	}
	acute := acuteSum / 7
	chronic := chronicSum / 28
	if chronic == 0 {
		return 0
	}
	return math.Round(acute/chronic*100) / 100
}

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
		{0.80, 0.90, 4}, // Edwards standard (was 5)
		{0.90, 1.01, 5}, // Edwards standard (was 10)
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

// CalculateMultiSport computes the PMC series for combined and per-sport-group in a single pass.
//
// dailyTSSBySport maps date strings (YYYY-MM-DD) to a map of sport_group → TSS.
// prevCombinedCTL/ATL seed the combined EWMA; prevSportCTL seeds per-sport EWMA (nil = all zero).
//
// Returns interleaved PMCDay slices: one "__combined__" row per day followed by one row per
// active sport group on that day. Sport CTL decays even on days with zero TSS.
//
// Sport-TSB semantics: uses the combined preUpdateATL (before today's EWMA step) to keep
// sport-specific readiness on the same scale as combined TSB (Banister convention).
func CalculateMultiSport(from, to time.Time, dailyTSSBySport map[string]map[string]float64, prevCombinedCTL, prevCombinedATL float64, prevSportCTL map[string]float64) []model.PMCDay {
	days := int(to.Sub(from).Hours()/24) + 1
	if days <= 0 {
		return nil
	}

	// Discover all sport groups present in the data.
	sgSet := make(map[string]struct{})
	for _, sportMap := range dailyTSSBySport {
		for sg := range sportMap {
			sgSet[sg] = struct{}{}
		}
	}
	// Stable-sorted slice of sport groups for deterministic output.
	sportGroups := make([]string, 0, len(sgSet))
	for sg := range sgSet {
		sportGroups = append(sportGroups, sg)
	}
	// Sort for determinism.
	for i := 1; i < len(sportGroups); i++ {
		for j := i; j > 0 && sportGroups[j] < sportGroups[j-1]; j-- {
			sportGroups[j], sportGroups[j-1] = sportGroups[j-1], sportGroups[j]
		}
	}

	// Initialise per-sport CTL state.
	sportCTL := make(map[string]float64, len(sportGroups))
	for _, sg := range sportGroups {
		if prevSportCTL != nil {
			sportCTL[sg] = prevSportCTL[sg]
		}
	}

	result := make([]model.PMCDay, 0, days*(1+len(sportGroups)))
	combinedCTL := prevCombinedCTL
	combinedATL := prevCombinedATL

	for i := range days {
		d := from.AddDate(0, 0, i)
		dateStr := d.Format("2006-01-02")

		sportMap := dailyTSSBySport[dateStr] // nil if no training

		// Total TSS across all sport groups for this day.
		var totalTSS float64
		for _, sg := range sportGroups {
			totalTSS += sportMap[sg]
		}

		// TSB = CTL_yesterday - ATL_yesterday (Banister: readiness BEFORE today's training).
		combinedTSB := combinedCTL - combinedATL

		// Save ATL before update — sport-TSB uses this so all sport rows share the same
		// pre-update baseline (avoids double-counting the day's fatigue bump).
		preUpdateATL := combinedATL

		// EWMA update for combined.
		combinedCTL = combinedCTL*ctlDecay + totalTSS*ctlFactor
		combinedATL = combinedATL*atlDecay + totalTSS*atlFactor

		result = append(result, model.PMCDay{
			Date:       dateStr,
			TSS:        math.Round(totalTSS*100) / 100,
			CTL:        math.Round(combinedCTL*100) / 100,
			ATL:        math.Round(combinedATL*100) / 100,
			TSB:        math.Round(combinedTSB*100) / 100,
			SportGroup: "__combined__",
		})

		// Per-sport rows (CTL decays even when TSS=0).
		for _, sg := range sportGroups {
			sportTSS := sportMap[sg] // 0 if sport not active today
			ctl := sportCTL[sg]

			sportTSB := ctl - preUpdateATL
			ctl = ctl*ctlDecay + sportTSS*ctlFactor
			sportCTL[sg] = ctl

			result = append(result, model.PMCDay{
				Date:       dateStr,
				TSS:        math.Round(sportTSS*100) / 100,
				CTL:        math.Round(ctl*100) / 100,
				ATL:        math.Round(preUpdateATL*100) / 100, // ATL is always combined
				TSB:        math.Round(sportTSB*100) / 100,
				SportGroup: sg,
			})
		}
	}

	return result
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

		// TSB = yesterday's CTL - yesterday's ATL (readiness at start of today,
		// before today's training). CTL/ATL are then updated with today's TSS.
		// This mixed temporal semantics is intentional (Banister convention).
		tsb := ctl - atl

		// Exponential weighted moving average
		ctl = ctl*ctlDecay + tss*ctlFactor
		atl = atl*atlDecay + tss*atlFactor

		result = append(result, model.PMCDay{
			Date: dateStr,
			TSS:  math.Round(tss*100) / 100,
			CTL:  math.Round(ctl*100) / 100,
			ATL:  math.Round(atl*100) / 100,
			TSB:  math.Round(tsb*100) / 100,
		})
	}

	return result
}
