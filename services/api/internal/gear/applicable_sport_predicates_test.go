package gear

import (
	"testing"
)

func TestComputeApplicableSportPredicatesReturnsCyclingForBike(t *testing.T) {
	preds, err := ComputeApplicableSportPredicates("bike", nil)
	if err != nil {
		t.Fatalf("ComputeApplicableSportPredicates(bike) error = %v", err)
	}
	if len(preds) != 1 {
		t.Fatalf("len = %d, want 1; got %#v", len(preds), preds)
	}
	if preds[0].SportType != "cycling" {
		t.Fatalf("SportType = %q, want cycling", preds[0].SportType)
	}
	if preds[0].SubType != nil {
		t.Fatalf("SubType = %v, want nil", preds[0].SubType)
	}
}

func TestComputeApplicableSportPredicatesForBikeMTBWithElectricModifier(t *testing.T) {
	modifiers := map[string]any{"is_electric": true}
	preds, err := ComputeApplicableSportPredicates("bike.mtb", modifiers)
	if err != nil {
		t.Fatalf("ComputeApplicableSportPredicates(bike.mtb, electric) error = %v", err)
	}
	if len(preds) == 0 {
		t.Fatal("expected at least one predicate, got none")
	}

	// Base predicate: cycling/mountain must be present.
	found := false
	for _, p := range preds {
		if p.SportType == "cycling" && p.SubType != nil && *p.SubType == "mountain" {
			found = true
			// Modifier overlay: is_electric should be set on the predicate.
			if p.Modifiers == nil || p.Modifiers.IsElectric == nil || !*p.Modifiers.IsElectric {
				t.Fatalf("predicate cycling/mountain has no is_electric modifier; got %#v", p.Modifiers)
			}
			break
		}
	}
	if !found {
		t.Fatalf("cycling/mountain predicate not found; got %#v", preds)
	}
}

func TestComputeApplicableSportPredicatesReturnsEmptyForUnknownType(t *testing.T) {
	preds, err := ComputeApplicableSportPredicates("unknown_type_xyz", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) != 0 {
		t.Fatalf("expected empty slice for unknown type, got %#v", preds)
	}
}

func TestComputeApplicableSportPredicatesIgnoresNonApplicableModifiers(t *testing.T) {
	// is_commute is not in GearApplicableModifierKeys; must be silently dropped.
	modifiers := map[string]any{"is_commute": true}
	preds, err := ComputeApplicableSportPredicates("bike", modifiers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range preds {
		if p.Modifiers != nil && p.Modifiers.IsCommute != nil {
			t.Fatalf("is_commute leaked into predicate modifiers: %#v", p.Modifiers)
		}
	}
}

func TestHelmetGogglesIncludeSnowboarding(t *testing.T) {
	for _, gearKey := range []string{"helmet", "goggles"} {
		preds, ok := applicableSportPredicatesDefaults[gearKey]
		if !ok {
			t.Fatalf("gear key %q not found in defaults map", gearKey)
		}
		found := false
		for _, p := range preds {
			if p.SportType == "snowboarding" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("gear key %q: snowboarding predicate missing; got %#v", gearKey, preds)
		}
	}
}

// --- Change 1: parent-prefix-fallback tests ---

// (a) Exact match still works.
func TestLookupBaseDefaults_ExactMatch(t *testing.T) {
	preds := lookupBaseDefaults("bike.road")
	if len(preds) == 0 {
		t.Fatal("expected predicates for bike.road, got none")
	}
	found := false
	for _, p := range preds {
		if p.SportType == "cycling" && p.SubType != nil && *p.SubType == "road" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cycling/road not found in bike.road predicates: %#v", preds)
	}
}

// (b) Parent-prefix fallback cases.
func TestLookupBaseDefaults_ParentFallback(t *testing.T) {
	tests := []struct {
		typeKey      string
		wantSport    string
		wantSubType  *string
	}{
		{"bike.road.endurance", "cycling", strPtr("road")},
		{"bike.mtb.xc", "cycling", strPtr("mountain")},
		{"bike.unicycle", "cycling", nil},
		{"shoes.cycling.indoor", "cycling", nil},
	}
	for _, tc := range tests {
		t.Run(tc.typeKey, func(t *testing.T) {
			preds := lookupBaseDefaults(tc.typeKey)
			if len(preds) == 0 {
				t.Fatalf("expected predicates via fallback for %q, got none", tc.typeKey)
			}
			found := false
			for _, p := range preds {
				subMatch := (tc.wantSubType == nil && p.SubType == nil) ||
					(tc.wantSubType != nil && p.SubType != nil && *tc.wantSubType == *p.SubType)
				if p.SportType == tc.wantSport && subMatch {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("predicate sport=%q subType=%v not found in fallback result for %q: %#v",
					tc.wantSport, tc.wantSubType, tc.typeKey, preds)
			}
		})
	}
}

// (d) Key without any ancestor in the map returns empty.
func TestLookupBaseDefaults_NoAncestorReturnsEmpty(t *testing.T) {
	// apparel.gloves: apparel not in map, no ancestor → empty
	preds := lookupBaseDefaults("apparel.gloves")
	if len(preds) != 0 {
		t.Fatalf("expected empty for apparel.gloves, got %#v", preds)
	}
}

// (e) Unknown key with no dots returns empty.
func TestLookupBaseDefaults_UnknownRootKeyReturnsEmpty(t *testing.T) {
	preds := lookupBaseDefaults("completely_unknown_xyz")
	if len(preds) != 0 {
		t.Fatalf("expected empty for unknown key, got %#v", preds)
	}
}

// (e) Empty typeKey returns empty.
func TestLookupBaseDefaults_EmptyKeyReturnsEmpty(t *testing.T) {
	preds := lookupBaseDefaults("")
	if len(preds) != 0 {
		t.Fatalf("expected empty for empty typeKey, got %#v", preds)
	}
}

// --- Change 2: explicit entries for non-bike exotics ---

// (c) Representative new explicit entries.
func TestLookupBaseDefaults_ExplicitNewEntries(t *testing.T) {
	tests := []struct {
		typeKey   string
		wantSports []string
	}{
		{"paddle", []string{"kayaking", "canoeing", "stand_up_paddleboarding", "rowing"}},
		{"dive_mask", []string{"diving", "snorkeling"}},
		{"snowboard", []string{"snowboarding"}},
		{"watercraft.kayak", []string{"kayaking"}},
		{"fishing_gear", []string{"fishing"}},
	}
	for _, tc := range tests {
		t.Run(tc.typeKey, func(t *testing.T) {
			preds := lookupBaseDefaults(tc.typeKey)
			if len(preds) == 0 {
				t.Fatalf("expected predicates for %q, got none", tc.typeKey)
			}
			got := make(map[string]bool, len(preds))
			for _, p := range preds {
				got[p.SportType] = true
			}
			for _, want := range tc.wantSports {
				if !got[want] {
					t.Errorf("sport_type %q missing from %q predicates: %v", want, tc.typeKey, preds)
				}
			}
			if len(preds) != len(tc.wantSports) {
				t.Errorf("%q: expected %d predicates, got %d: %#v", tc.typeKey, len(tc.wantSports), len(preds), preds)
			}
		})
	}
}

// ComputeApplicableSportPredicates must also use the fallback.
func TestComputeApplicableSportPredicates_FallbackViaPublicFunc(t *testing.T) {
	preds, err := ComputeApplicableSportPredicates("bike.road.endurance", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) == 0 {
		t.Fatal("expected predicates via fallback for bike.road.endurance, got none")
	}
}

// (f) Modifier overlay applies on top of a fallback-resolved predicate.
func TestComputeApplicableSportPredicates_ModifierOverlayOnFallback(t *testing.T) {
	// bike.road.endurance falls back to bike.road → cycling/road.
	// Adding is_electric modifier must appear on the returned predicates.
	preds, err := ComputeApplicableSportPredicates("bike.road.endurance", map[string]any{"is_electric": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) == 0 {
		t.Fatal("expected at least one predicate")
	}
	for _, p := range preds {
		if p.Modifiers == nil || p.Modifiers.IsElectric == nil || !*p.Modifiers.IsElectric {
			t.Fatalf("is_electric modifier not applied on fallback predicate: %#v", p)
		}
	}
}
