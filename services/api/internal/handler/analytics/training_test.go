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
// Verify that acwrStatus returns the correct buckets.
// ---------------------------------------------------------------------------

func TestGetPMCACWRCalculation(t *testing.T) {
	tests := []struct {
		acwr     float64
		expected string
	}{
		{0.5, "fresh"},
		{0.79, "fresh"},
		{0.8, "optimal"},
		{0.99, "optimal"},
		{1.0, "overreach"},
		{1.3, "overreach"},
		{1.31, "danger"},
		{1.5, "danger"},
	}
	for _, tc := range tests {
		got := acwrStatus(tc.acwr)
		if got != tc.expected {
			t.Errorf("acwrStatus(%.2f) = %q, want %q", tc.acwr, got, tc.expected)
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

	// Known values: [0, 100] → mean=50, variance=((−50)²+(50)²)/2=2500, stddev=50, monotony=1.0
	tss2 := []float64{0, 100}
	m2 := calcMonotony(tss2)
	if math.Abs(m2-1.0) > 0.001 {
		t.Errorf("expected monotony ~1.0, got %f", m2)
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
// TestGetPMCProjection
// Verify that linear projection extrapolates a perfect linear CTL series.
// ---------------------------------------------------------------------------

func TestGetPMCProjection(t *testing.T) {
	// Build 30 days of PMC data with CTL increasing by 1 per day starting at 50.
	days := make([]model.PMCDay, 30)
	base := 50.0
	for i := range days {
		days[i] = model.PMCDay{
			Date: "2025-01-" + pad2(i+1),
			CTL:  base + float64(i),
		}
	}

	proj := linearProjection(days, 28, 7)
	if len(proj) != 7 {
		t.Fatalf("expected 7 projection days, got %d", len(proj))
	}

	// The last real CTL is days[29].CTL = 50+29 = 79.
	// Each projected day should increase by ~1.
	for i, p := range proj {
		expected := days[29].CTL + float64(i+1)
		if math.Abs(p.CTL-expected) > 0.5 {
			t.Errorf("projection day %d: CTL = %.2f, want ~%.2f", i, p.CTL, expected)
		}
	}
}

// ---------------------------------------------------------------------------
// TestGetPMCProjectionInsufficientData
// Projection with fewer than 2 data points returns empty slice.
// ---------------------------------------------------------------------------

func TestGetPMCProjectionInsufficientData(t *testing.T) {
	proj := linearProjection([]model.PMCDay{
		{Date: "2025-01-01", CTL: 40},
	}, 28, 28)
	if len(proj) != 0 {
		t.Errorf("expected empty projection for 1 day, got %d", len(proj))
	}

	proj2 := linearProjection([]model.PMCDay{}, 28, 28)
	if len(proj2) != 0 {
		t.Errorf("expected empty projection for 0 days, got %d", len(proj2))
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
