package handler

// enrichment_clamp_test.go — Unit tests for the PATCH-clamp guards on
// model.EnrichmentSettings.SummitDetectionRadiusM.
//
// The clamp is applied in UpdateEnrichmentSettings (enrichment.go) after the
// JSON-merge step, before the DB write.  These tests replicate the clamp
// logic locally (same copy-pattern as enrichment_merge_test.go) so they run
// without any DB or HTTP infrastructure.

import (
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// clampEnrichmentSettingsForTest replicates the clamp guards from
// Handler.UpdateEnrichmentSettings.  Any change to those guards must be
// mirrored here (the tests below will then catch regressions).
func clampEnrichmentSettingsForTest(s *model.EnrichmentSettings) {
	if math.IsNaN(s.SummitDetectionRadiusM) || math.IsInf(s.SummitDetectionRadiusM, 0) {
		s.SummitDetectionRadiusM = 80
	}
	if s.SummitDetectionRadiusM < 10 {
		s.SummitDetectionRadiusM = 10
	}
	if s.SummitDetectionRadiusM > 150 {
		s.SummitDetectionRadiusM = 150
	}
}

// TestEnrichmentSettingsPATCH_SummitRadius_ClampLow verifies that a value
// below the minimum (10 m) is clamped up to 10.
func TestEnrichmentSettingsPATCH_SummitRadius_ClampLow(t *testing.T) {
	s := &model.EnrichmentSettings{SummitDetectionRadiusM: 5}
	clampEnrichmentSettingsForTest(s)
	if s.SummitDetectionRadiusM != 10 {
		t.Errorf("SummitDetectionRadiusM = %v, want 10 (clamped from 5)", s.SummitDetectionRadiusM)
	}
}

// TestEnrichmentSettingsPATCH_SummitRadius_ClampHigh verifies that a value
// above the maximum (150 m) is clamped down to 150.
func TestEnrichmentSettingsPATCH_SummitRadius_ClampHigh(t *testing.T) {
	s := &model.EnrichmentSettings{SummitDetectionRadiusM: 200}
	clampEnrichmentSettingsForTest(s)
	if s.SummitDetectionRadiusM != 150 {
		t.Errorf("SummitDetectionRadiusM = %v, want 150 (clamped from 200)", s.SummitDetectionRadiusM)
	}
}

// TestEnrichmentSettingsPATCH_SummitRadius_NaN verifies that a NaN value
// (which can arrive from JSON "NaN" strings or floating-point edge cases)
// is replaced with the default of 80.
func TestEnrichmentSettingsPATCH_SummitRadius_NaN(t *testing.T) {
	s := &model.EnrichmentSettings{SummitDetectionRadiusM: math.NaN()}
	clampEnrichmentSettingsForTest(s)
	if s.SummitDetectionRadiusM != 80 {
		t.Errorf("SummitDetectionRadiusM = %v, want 80 (default for NaN)", s.SummitDetectionRadiusM)
	}
}
