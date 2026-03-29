package analytics

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ---------------------------------------------------------------------------
// TestGetPMCACWRCalculation
// Verify that getAcwrStatus returns the correct buckets.
// ---------------------------------------------------------------------------

func TestGetPMCACWRCalculation(t *testing.T) {
	tests := []struct {
		acwr     float64
		expected string
	}{
		{0.5, "reduced"},
		{0.79, "reduced"},
		{0.8, "optimal"},
		{1.0, "optimal"},
		{1.29, "optimal"},
		{1.3, "overreach"},
		{1.5, "overreach"},
		{1.51, "danger"},
		{2.0, "danger"},
	}
	for _, tc := range tests {
		got := getAcwrStatus(tc.acwr)
		if got != tc.expected {
			t.Errorf("getAcwrStatus(%.2f) = %q, want %q", tc.acwr, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// TestGetPMCMonotony
// Verify monotony calculation: stddev / mean of a TSS slice.
// ---------------------------------------------------------------------------

func TestGetPMCMonotony(t *testing.T) {
	// All same values → stddev = 0 → monotony = 0.
	tss := []float64{100, 100, 100, 100, 100, 100, 100}
	m := calcMonotony(tss)
	if m != 0 {
		t.Errorf("expected monotony 0 for uniform TSS, got %f", m)
	}

	// Known values: [0, 100] → mean=50, sample variance=((−50)²+(50)²)/1=5000, stddev=~70.71
	// monotony = mean/stddev = 50/70.71 ≈ 0.7071
	tss2 := []float64{0, 100}
	m2 := calcMonotony(tss2)
	if math.Abs(m2-0.7071) > 0.001 {
		t.Errorf("expected monotony ~0.7071, got %f", m2)
	}

	// Fewer than 2 values → 0.
	if calcMonotony([]float64{100}) != 0 {
		t.Errorf("expected 0 for single element")
	}
	if calcMonotony([]float64{}) != 0 {
		t.Errorf("expected 0 for empty slice")
	}

	// All zeros → mean = 0 → monotony = 0 (avoid division by zero).
	if calcMonotony([]float64{0, 0, 0}) != 0 {
		t.Errorf("expected 0 for all-zero TSS")
	}
}

// ---------------------------------------------------------------------------
// TestEwmaProjection
// Verify that ewmaProjection returns correct number of days and decays CTL/ATL
// correctly given a known avgTSS.
// ---------------------------------------------------------------------------

func TestEwmaProjection(t *testing.T) {
	// Build 10 days of PMC data with constant TSS=100, CTL=50, ATL=60.
	days := make([]model.PMCDay, 10)
	for i := range days {
		days[i] = model.PMCDay{
			Date: "2025-01-" + pad2(i+1),
			TSS:  100,
			CTL:  50,
			ATL:  60,
		}
	}

	proj := ewmaProjection(days, 7)
	if len(proj) != 7 {
		t.Fatalf("expected 7 projection days, got %d", len(proj))
	}

	// First projected date must be day after last input day.
	if proj[0].Date != "2025-01-11" {
		t.Errorf("expected first projection date 2025-01-11, got %s", proj[0].Date)
	}

	// With avgTSS=100, CTL=50, ATL=60:
	// ctlDecay = exp(-1/42), ctlFactor = 1 - ctlDecay
	// Day 1 CTL = 50*ctlDecay + 100*ctlFactor — must be between 50 and 100.
	if proj[0].CTL <= 50 || proj[0].CTL >= 100 {
		t.Errorf("unexpected CTL on day 1: %f", proj[0].CTL)
	}

	// TSB on day 1 = CTL_last - ATL_last = 50 - 60 = -10.
	if math.Abs(proj[0].TSB-(-10)) > 0.01 {
		t.Errorf("expected TSB -10 on day 1, got %f", proj[0].TSB)
	}
}

// ---------------------------------------------------------------------------
// TestEwmaProjectionInsufficientData
// Projection with fewer than 7 data points returns nil.
// ---------------------------------------------------------------------------

func TestEwmaProjectionInsufficientData(t *testing.T) {
	proj := ewmaProjection([]model.PMCDay{
		{Date: "2025-01-01", CTL: 40, ATL: 45, TSS: 80},
	}, 28)
	if proj != nil {
		t.Errorf("expected nil projection for 1 day, got %d items", len(proj))
	}

	proj2 := ewmaProjection([]model.PMCDay{}, 28)
	if proj2 != nil {
		t.Errorf("expected nil projection for 0 days, got %d items", len(proj2))
	}
}

// ---------------------------------------------------------------------------
// TestEarlyReturnOnNoUserID
// Handlers must return 401 when no user identity is present in context.
// ---------------------------------------------------------------------------

func TestEarlyReturnOnNoUserID(t *testing.T) {
	h := &Handler{DB: nil} // DB is nil; should never be reached when userID is missing.

	handlers := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"GetPMC", h.GetPMC},
		{"GetYearProgression", h.GetYearProgression},
		{"GetTrainingVolume", h.GetTrainingVolume},
		{"GetZoneDistribution", h.GetZoneDistribution},
		{"GetPersonalRecords", h.GetPersonalRecords},
	}

	for _, hh := range handlers {
		t.Run(hh.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/analytics/"+hh.name, nil)
			// No user ID injected into context → GetUserID returns "".
			rr := httptest.NewRecorder()
			hh.fn(rr, req)
			if rr.Code != 401 {
				t.Errorf("%s: expected 401, got %d", hh.name, rr.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pad2 returns a zero-padded 2-digit string for integers 1–31.
func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
