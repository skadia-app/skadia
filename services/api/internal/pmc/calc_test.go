package pmc

import (
	"math"
	"testing"
	"time"
)

func TestCalculateMultiSport_BasicShape(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	dailyTSSBySport := map[string]map[string]float64{
		"2026-01-02": {"cycling": 80, "running": 40},
		"2026-01-05": {"cycling": 60},
		"2026-01-08": {"running": 50},
	}

	result := CalculateMultiSport(from, to, dailyTSSBySport, 0, 0, nil)

	// 10 days × (1 combined + 2 sport groups) = 30 rows
	if len(result) != 30 {
		t.Fatalf("expected 30 rows (10 days × 3), got %d", len(result))
	}
}

func TestCalculateMultiSport_CombinedTSSMatchesSum(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	dailyTSSBySport := map[string]map[string]float64{
		"2026-01-02": {"cycling": 80, "running": 40},
	}

	result := CalculateMultiSport(from, to, dailyTSSBySport, 0, 0, nil)

	// Find the combined row for 2026-01-02.
	var combinedTSS float64
	for _, d := range result {
		if d.Date == "2026-01-02" && d.SportGroup == "__combined__" {
			combinedTSS = d.TSS
		}
	}
	if math.Abs(combinedTSS-120) > 0.01 {
		t.Errorf("combined TSS on 2026-01-02 = %.2f, want 120", combinedTSS)
	}
}

func TestCalculateMultiSport_SportCTLExists(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	dailyTSSBySport := map[string]map[string]float64{
		"2026-01-02": {"cycling": 80, "running": 40},
		"2026-01-05": {"cycling": 60},
		"2026-01-08": {"running": 50},
	}

	result := CalculateMultiSport(from, to, dailyTSSBySport, 0, 0, nil)

	// After Jan-10, find cycling CTL > 0 and running CTL > 0.
	var cyclingCTL, runningCTL float64
	for _, d := range result {
		if d.Date == "2026-01-10" {
			switch d.SportGroup {
			case "cycling":
				cyclingCTL = d.CTL
			case "running":
				runningCTL = d.CTL
			}
		}
	}
	if cyclingCTL <= 0 {
		t.Errorf("cycling CTL should be > 0 after training, got %.4f", cyclingCTL)
	}
	if runningCTL <= 0 {
		t.Errorf("running CTL should be > 0 after training, got %.4f", runningCTL)
	}
}

func TestCalculateMultiSport_SportTSBUsesPreUpdateATL(t *testing.T) {
	// Verify sport-TSB = sportCTL_yesterday - combinedATL_yesterday (preUpdateATL).
	// Day 1: combined TSS=100 from cycling only (ATL starts 0).
	// Day 2: no training.
	// On day 2: preUpdateATL = combinedATL after day 1.
	// Sport TSB for day 2 cycling = sportCTL[after day1] - preUpdateATL[day2].
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	dailyTSSBySport := map[string]map[string]float64{
		"2026-01-01": {"cycling": 100},
	}

	result := CalculateMultiSport(from, to, dailyTSSBySport, 0, 0, nil)

	// Locate day 2 cycling row.
	var day2CyclingTSB, day2CombinedATL float64
	var day2CyclingCTL float64
	var day1CombinedATL float64

	for _, d := range result {
		if d.Date == "2026-01-01" && d.SportGroup == "__combined__" {
			day1CombinedATL = d.ATL
		}
		if d.Date == "2026-01-02" && d.SportGroup == "cycling" {
			day2CyclingTSB = d.TSB
			day2CombinedATL = d.ATL
			day2CyclingCTL = d.CTL
		}
	}

	// preUpdateATL for day 2 = day1 combined ATL (since day2 has no training).
	// sport cycling CTL after day 1 = 100 * ctlFactor (first EWMA step from 0).
	// sportCTL before day 2 update = that value.
	// expectedTSB = sportCTL_day1 - day1CombinedATL.
	_ = day2CombinedATL // ATL field on sport rows is preUpdateATL
	expectedTSB := day2CyclingCTL/ctlDecay - day1CombinedATL

	// day2CyclingCTL = sportCTL_day1 * ctlDecay (no TSS on day 2)
	// → sportCTL_day1 = day2CyclingCTL / ctlDecay
	if math.Abs(day2CyclingTSB-expectedTSB) > 0.01 {
		t.Errorf("sport TSB on day2 = %.4f, want %.4f (sportCTL_prev - preUpdateATL)", day2CyclingTSB, expectedTSB)
	}
}

func TestCalculateMultiSport_SportCTLDecaysOnZeroTSS(t *testing.T) {
	// After a training day, CTL should decay on subsequent rest days.
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	dailyTSSBySport := map[string]map[string]float64{
		"2026-01-01": {"cycling": 100},
	}

	result := CalculateMultiSport(from, to, dailyTSSBySport, 0, 0, nil)

	var ctl1, ctl5 float64
	for _, d := range result {
		if d.SportGroup == "cycling" {
			switch d.Date {
			case "2026-01-01":
				ctl1 = d.CTL
			case "2026-01-05":
				ctl5 = d.CTL
			}
		}
	}

	if ctl5 >= ctl1 {
		t.Errorf("cycling CTL should decay: day1=%.4f, day5=%.4f", ctl1, ctl5)
	}
}

func TestCalculateMultiSport_EmptyInput(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	result := CalculateMultiSport(from, to, nil, 0, 0, nil)

	// No sport groups → only 3 combined rows.
	if len(result) != 3 {
		t.Fatalf("empty input should yield 3 combined rows, got %d", len(result))
	}
	for _, d := range result {
		if d.SportGroup != "__combined__" {
			t.Errorf("expected __combined__ rows only, got %q", d.SportGroup)
		}
		if d.TSS != 0 || d.CTL != 0 || d.ATL != 0 {
			t.Errorf("all values should be 0 for empty input, got %+v", d)
		}
	}
}

func TestCalculateMultiSport_InvalidRange(t *testing.T) {
	from := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // to < from

	result := CalculateMultiSport(from, to, nil, 0, 0, nil)
	if result != nil {
		t.Errorf("invalid range should return nil, got %d rows", len(result))
	}
}

func TestCalculateMultiSport_CombinedCTLMatchesCalculate(t *testing.T) {
	// The combined CTL/ATL/TSB from CalculateMultiSport must match Calculate
	// when using the aggregated daily TSS.
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	dailyTSSBySport := map[string]map[string]float64{
		"2026-01-02": {"cycling": 80, "running": 40},
		"2026-01-05": {"cycling": 60},
		"2026-01-08": {"running": 50},
	}

	// Build combined daily TSS for comparison.
	dailyTSS := map[string]float64{
		"2026-01-02": 120,
		"2026-01-05": 60,
		"2026-01-08": 50,
	}

	multiResult := CalculateMultiSport(from, to, dailyTSSBySport, 0, 0, nil)
	singleResult := Calculate(from, to, dailyTSS, 0, 0)

	// Extract combined rows from multiResult.
	combinedRows := make(map[string]struct{ CTL, ATL, TSB float64 })
	for _, d := range multiResult {
		if d.SportGroup == "__combined__" {
			combinedRows[d.Date] = struct{ CTL, ATL, TSB float64 }{d.CTL, d.ATL, d.TSB}
		}
	}

	for _, d := range singleResult {
		c, ok := combinedRows[d.Date]
		if !ok {
			t.Errorf("missing combined row for date %s", d.Date)
			continue
		}
		if math.Abs(c.CTL-d.CTL) > 0.001 {
			t.Errorf("CTL mismatch on %s: multi=%.4f single=%.4f", d.Date, c.CTL, d.CTL)
		}
		if math.Abs(c.ATL-d.ATL) > 0.001 {
			t.Errorf("ATL mismatch on %s: multi=%.4f single=%.4f", d.Date, c.ATL, d.ATL)
		}
		if math.Abs(c.TSB-d.TSB) > 0.001 {
			t.Errorf("TSB mismatch on %s: multi=%.4f single=%.4f", d.Date, c.TSB, d.TSB)
		}
	}
}
