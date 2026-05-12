package handler

// enrichment_clamp_test.go — Unit tests for clampSummitDetectionRadius,
// the package-level helper called by UpdateEnrichmentSettings after the
// JSON-merge step, before the DB write.
//
// These tests exercise the exact same function the handler calls — no
// replica, no divergence risk.

import (
	"math"
	"testing"
)

// TestEnrichmentSettingsPATCH_SummitRadius_ClampLow verifies that a value
// below the minimum (10 m) is clamped up to 10.
// Testet den Helper, der vom Handler aufgerufen wird — identische Logik.
func TestEnrichmentSettingsPATCH_SummitRadius_ClampLow(t *testing.T) {
	got := clampSummitDetectionRadius(5)
	if got != 10 {
		t.Errorf("clampSummitDetectionRadius(5) = %v, want 10", got)
	}
}

// TestEnrichmentSettingsPATCH_SummitRadius_ClampHigh verifies that a value
// above the maximum (150 m) is clamped down to 150.
// Testet den Helper, der vom Handler aufgerufen wird — identische Logik.
func TestEnrichmentSettingsPATCH_SummitRadius_ClampHigh(t *testing.T) {
	got := clampSummitDetectionRadius(200)
	if got != 150 {
		t.Errorf("clampSummitDetectionRadius(200) = %v, want 150", got)
	}
}

// TestEnrichmentSettingsPATCH_SummitRadius_NaN verifies that a NaN value
// (which can arrive from JSON "NaN" strings or floating-point edge cases)
// is replaced with the default of 80.
// Testet den Helper, der vom Handler aufgerufen wird — identische Logik.
func TestEnrichmentSettingsPATCH_SummitRadius_NaN(t *testing.T) {
	got := clampSummitDetectionRadius(math.NaN())
	if got != 80 {
		t.Errorf("clampSummitDetectionRadius(NaN) = %v, want 80", got)
	}
}

func TestEnrichmentSettingsPATCH_AutoTitleDisplayPolicyValidation(t *testing.T) {
	valid := []string{"source_unless_generic", "source_first", "auto_first"}
	for _, policy := range valid {
		if !isValidAutoTitleDisplayPolicy(policy) {
			t.Fatalf("isValidAutoTitleDisplayPolicy(%q) = false, want true", policy)
		}
	}
	if isValidAutoTitleDisplayPolicy("always_generated") {
		t.Fatal("isValidAutoTitleDisplayPolicy(always_generated) = true, want false")
	}
}
