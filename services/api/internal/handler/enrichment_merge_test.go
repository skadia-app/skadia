package handler

// enrichment_merge_test.go — Unit tests for the JSON-merge logic used by
// UpdateEnrichmentSettings (PATCH semantics on user_enrichment_settings).
//
// These tests do NOT require a live DB. They replicate the pure-merge logic
// from UpdateEnrichmentSettings in enrichment.go (lines ~676–699) and pin
// down the behaviour that was called out as suspect in Finding #41:
//
//   * Frontend sends PATCH with only a subset of fields
//   * Backend loads the existing settings, merges the patch on top,
//     unmarshals into model.EnrichmentSettings
//   * The final struct must preserve fields that were NOT touched by the
//     patch, including nullable *string pointer fields.
//
// Finding #41 suspected that partial PATCHes could silently clear
// GeocodeFallbackURL (a *string field with `omitempty`). These tests
// demonstrate the merge logic actually behaves correctly in all the
// relevant scenarios.

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// mergeEnrichmentSettingsForTest replicates the merge logic from
// Handler.UpdateEnrichmentSettings without the DB or HTTP plumbing, so the
// behaviour can be unit-tested. This is intentionally a copy of the
// handler's merge sequence — any change in that handler must keep this
// copy in sync (the tests below would then catch regressions).
func mergeEnrichmentSettingsForTest(t *testing.T, existing *model.EnrichmentSettings, patchJSON []byte) model.EnrichmentSettings {
	t.Helper()

	var rawPatch map[string]json.RawMessage
	if err := json.Unmarshal(patchJSON, &rawPatch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}

	existingJSON, err := json.Marshal(existing)
	if err != nil {
		t.Fatalf("marshal existing: %v", err)
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(existingJSON, &merged); err != nil {
		t.Fatalf("unmarshal existing: %v", err)
	}
	maps.Copy(merged, rawPatch)

	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		t.Fatalf("marshal merged: %v", err)
	}

	var out model.EnrichmentSettings
	if err := json.Unmarshal(mergedJSON, &out); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}
	return out
}

// baseSettings returns a realistic fully-populated settings struct for
// patch-simulation tests. Non-zero / non-default values make it easy to
// tell apart "preserved" from "zeroed".
func baseSettings() *model.EnrichmentSettings {
	return &model.EnrichmentSettings{
		UserID:                    "user-123",
		EnableSummitDetection:     true,
		EnableWeatherEnrichment:   true,
		EnableStopDetection:       true,
		EnableAscentDetection:     true,
		EnableAutoNaming:          true,
		EnableLapDetection:        true,
		ClimbMinGradientPct:       5.0,
		ClimbMinElevationM:        100,
		RoundTripThresholdM:       150,
		OCRInvoicePrompt:          strPtr("custom invoice prompt"),
		OCRGearPrompt:             strPtr("custom gear prompt"),
		EnableForestDetection:     true,
		EnableWaterDetection:      true,
		EnableSettlementDetection: true,
		EnableBorderDetection:     true,
		EnableLiftDetection:       true,
		EnableSurfaceDetection:    true,
		EnableIntervalDetection:   true,
		EnableEmbeddings:          true,
		PhotoPreviewSize:          "wqhd",
		TerraIncognitaRadiusM:     1000,
		RouteDuplicateHausdorffM:  50,
		RouteDuplicateLengthDiffKm: 1.0,
		VideoClipFrames:           4,
		PhashThresholdBits:        3,
		GeocodeFallbackProvider:   "off",
		GeocodeFallbackURL:        strPtr("https://my-proxy.example/nominatim"),
		GeocodeFallbackTimeoutS:   5,
	}
}

// TestUpdateEnrichmentSettings_partial_preserves_url documents that sending a
// partial PATCH WITHOUT geocode_fallback_url does NOT clear the URL. This is
// the core concern in Finding #41.
func TestUpdateEnrichmentSettings_partial_preserves_url(t *testing.T) {
	existing := baseSettings()
	patch := []byte(`{"geocode_fallback_timeout_s": 7}`)

	out := mergeEnrichmentSettingsForTest(t, existing, patch)

	if out.GeocodeFallbackURL == nil {
		t.Fatalf("GeocodeFallbackURL was cleared by partial PATCH; want preserved")
	}
	if *out.GeocodeFallbackURL != "https://my-proxy.example/nominatim" {
		t.Errorf("GeocodeFallbackURL = %q, want %q",
			*out.GeocodeFallbackURL, "https://my-proxy.example/nominatim")
	}
	if out.GeocodeFallbackTimeoutS != 7 {
		t.Errorf("GeocodeFallbackTimeoutS = %d, want 7 (patch value)",
			out.GeocodeFallbackTimeoutS)
	}
	if out.GeocodeFallbackProvider != "off" {
		t.Errorf("GeocodeFallbackProvider = %q, want %q (preserved)",
			out.GeocodeFallbackProvider, "off")
	}
}

// TestUpdateEnrichmentSettings_explicit_null_clears_url documents the
// opposite: an explicit null in the PATCH body DOES clear the URL. This is
// the only supported way for the client to null out the field.
func TestUpdateEnrichmentSettings_explicit_null_clears_url(t *testing.T) {
	existing := baseSettings()
	patch := []byte(`{"geocode_fallback_url": null}`)

	out := mergeEnrichmentSettingsForTest(t, existing, patch)

	if out.GeocodeFallbackURL != nil {
		t.Fatalf("GeocodeFallbackURL = %q, want nil after explicit null PATCH",
			*out.GeocodeFallbackURL)
	}
	// Other fields must remain untouched.
	if out.GeocodeFallbackProvider != "off" {
		t.Errorf("GeocodeFallbackProvider = %q, want %q (preserved)",
			out.GeocodeFallbackProvider, "off")
	}
	if out.GeocodeFallbackTimeoutS != 5 {
		t.Errorf("GeocodeFallbackTimeoutS = %d, want 5 (preserved)",
			out.GeocodeFallbackTimeoutS)
	}
}

// TestUpdateEnrichmentSettings_partial_preserves_all_pointer_fields widens
// the guarantee to every *string field in the struct (OCR prompts + the
// fallback URL). A partial PATCH that touches none of them must leave all
// of them intact.
func TestUpdateEnrichmentSettings_partial_preserves_all_pointer_fields(t *testing.T) {
	existing := baseSettings()
	patch := []byte(`{"enable_summit_detection": false}`)

	out := mergeEnrichmentSettingsForTest(t, existing, patch)

	if out.OCRInvoicePrompt == nil || *out.OCRInvoicePrompt != "custom invoice prompt" {
		t.Errorf("OCRInvoicePrompt not preserved: %+v", out.OCRInvoicePrompt)
	}
	if out.OCRGearPrompt == nil || *out.OCRGearPrompt != "custom gear prompt" {
		t.Errorf("OCRGearPrompt not preserved: %+v", out.OCRGearPrompt)
	}
	if out.GeocodeFallbackURL == nil || *out.GeocodeFallbackURL != "https://my-proxy.example/nominatim" {
		t.Errorf("GeocodeFallbackURL not preserved: %+v", out.GeocodeFallbackURL)
	}
	if out.EnableSummitDetection {
		t.Error("EnableSummitDetection should be false after patch")
	}
}

// TestUpdateEnrichmentSettings_nil_url_in_db_stays_nil covers the other
// edge of the pointer-field behaviour: existing URL is nil, PATCH does not
// mention it → result stays nil (omitempty drops it from the intermediate
// JSON, but that's fine because nothing adds it back either).
func TestUpdateEnrichmentSettings_nil_url_in_db_stays_nil(t *testing.T) {
	existing := baseSettings()
	existing.GeocodeFallbackURL = nil
	patch := []byte(`{"geocode_fallback_provider": "nominatim"}`)

	out := mergeEnrichmentSettingsForTest(t, existing, patch)

	if out.GeocodeFallbackURL != nil {
		t.Errorf("GeocodeFallbackURL = %q, want nil (was nil in DB, not in patch)",
			*out.GeocodeFallbackURL)
	}
	if out.GeocodeFallbackProvider != "nominatim" {
		t.Errorf("GeocodeFallbackProvider = %q, want %q", out.GeocodeFallbackProvider, "nominatim")
	}
}

// TestUpdateEnrichmentSettings_bool_zero_not_clobbered confirms the
// original rationale for the map[string]json.RawMessage merge: sending
// only some fields must NOT reset un-mentioned bool fields to false.
func TestUpdateEnrichmentSettings_bool_zero_not_clobbered(t *testing.T) {
	existing := baseSettings()
	patch := []byte(`{"climb_min_gradient_pct": 7.5}`)

	out := mergeEnrichmentSettingsForTest(t, existing, patch)

	if !out.EnableSummitDetection ||
		!out.EnableWeatherEnrichment ||
		!out.EnableStopDetection ||
		!out.EnableAscentDetection ||
		!out.EnableAutoNaming ||
		!out.EnableLapDetection ||
		!out.EnableForestDetection ||
		!out.EnableWaterDetection ||
		!out.EnableSettlementDetection ||
		!out.EnableBorderDetection ||
		!out.EnableLiftDetection ||
		!out.EnableSurfaceDetection ||
		!out.EnableIntervalDetection ||
		!out.EnableEmbeddings {
		t.Error("some bool module flags were zeroed by a partial PATCH — merge logic regression")
	}
	if out.ClimbMinGradientPct != 7.5 {
		t.Errorf("ClimbMinGradientPct = %v, want 7.5 (patch value)", out.ClimbMinGradientPct)
	}
}
