package gear_test

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/gear"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func boolPtr(v bool) *bool { return &v }

func assertBoolPatch(t *testing.T, field string, got, want *bool) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s patch = %v, want nil", field, *got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s patch = nil, want %v", field, *want)
	}
	if *got != *want {
		t.Fatalf("%s patch = %v, want %v", field, *got, *want)
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice length = %d, want %d; got %#v want %#v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- Migrated from internal/sport/normalize_test.go alongside the gear-logic
// move (Task 1.4). Behavior unchanged; identifiers requalified gear./sport. ---

func TestGearDefaultPredicateForSourcePreservesSubtypeAndModifiers(t *testing.T) {
	tests := []struct {
		raw          string
		wantSport    string
		wantSubType  string
		wantElectric *bool
		wantVirtual  *bool
	}{
		{raw: "Mountain Bike Ride", wantSport: "cycling", wantSubType: "mountain"},
		{raw: "E-Mountain Bike Ride", wantSport: "cycling", wantSubType: "mountain", wantElectric: boolPtr(true)},
		{raw: "Virtual Ride", wantSport: "cycling", wantVirtual: boolPtr(true)},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := gear.GearDefaultPredicateForSource(sport.SourceStrava, tt.raw)
			if !ok {
				t.Fatalf("GearDefaultPredicateForSource(SourceStrava, %q) returned ok=false", tt.raw)
			}
			if got.SportType != tt.wantSport {
				t.Fatalf("SportType = %q, want %q", got.SportType, tt.wantSport)
			}
			gotSubType := ""
			if got.SubType != nil {
				gotSubType = *got.SubType
			}
			if gotSubType != tt.wantSubType {
				t.Fatalf("SubType = %q, want %q", gotSubType, tt.wantSubType)
			}
			var gotElectric, gotVirtual *bool
			if got.Modifiers != nil {
				gotElectric = got.Modifiers.IsElectric
				gotVirtual = got.Modifiers.IsVirtual
			}
			assertBoolPatch(t, "is_electric", gotElectric, tt.wantElectric)
			assertBoolPatch(t, "is_virtual", gotVirtual, tt.wantVirtual)
		})
	}
}

func TestGearDefaultPredicateForSourceUnknownStravaSource(t *testing.T) {
	predicate, ok := gear.GearDefaultPredicateForSource(sport.SourceStrava, "Definitely Not A Known Sport")
	if ok || predicate.SportType != "" {
		t.Fatalf("GearDefaultPredicateForSource unknown Strava = %#v, %v; want empty, false", predicate, ok)
	}
}

func TestNormalizeGearDefaultPredicates(t *testing.T) {
	subType := "mountain"
	got, err := gear.NormalizeGearDefaultPredicates([]gear.GearDefaultPredicate{
		{
			SportType: " cycling ",
			SubType:   &subType,
			Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)},
		},
		{
			SportType: "cycling",
			SubType:   &subType,
			Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)},
		},
		{SportType: "fitness_equipment"},
	})
	if err != nil {
		t.Fatalf("NormalizeGearDefaultPredicates() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].SportType != "cycling" || got[0].SubType == nil || *got[0].SubType != "mountain" {
		t.Fatalf("first predicate = %#v, want cycling mountain", got[0])
	}
	if got[0].Modifiers == nil {
		t.Fatal("first predicate modifiers = nil, want electric modifier")
	}
	assertBoolPatch(t, "is_electric", got[0].Modifiers.IsElectric, boolPtr(true))
	if got[1].SportType != "fitness_equipment" || got[1].SubType != nil || got[1].Modifiers != nil {
		t.Fatalf("second predicate = %#v, want broad fitness_equipment", got[1])
	}
}

func TestNormalizeGearDefaultPredicatesRejectsUnknown(t *testing.T) {
	if _, err := gear.NormalizeGearDefaultPredicates([]gear.GearDefaultPredicate{{SportType: "not_a_sport"}}); err == nil {
		t.Fatal("NormalizeGearDefaultPredicates accepted unknown sport")
	}
}

func TestNormalizeGearDefaultPredicatesRejectsCommuteModifier(t *testing.T) {
	if _, err := gear.NormalizeGearDefaultPredicates([]gear.GearDefaultPredicate{{
		SportType: "cycling",
		Modifiers: &sport.ModifierPatch{IsCommute: boolPtr(true)},
	}}); err == nil {
		t.Fatal("NormalizeGearDefaultPredicates accepted commute modifier")
	}
}

func TestGearDefaultPredicateMatchesActivity(t *testing.T) {
	mountain := "mountain"
	predicate := gear.GearDefaultPredicate{
		SportType: "cycling",
		SubType:   &mountain,
		Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)},
	}
	if !gear.GearDefaultPredicateMatchesActivity(predicate, "cycling", "mountain", sport.ModifierValues{IsElectric: true}) {
		t.Fatal("predicate did not match electric mountain cycling")
	}
	if gear.GearDefaultPredicateMatchesActivity(predicate, "cycling", "mountain", sport.ModifierValues{IsElectric: false}) {
		t.Fatal("predicate matched non-electric mountain cycling")
	}
	if gear.GearDefaultPredicateMatchesActivity(predicate, "cycling", "road", sport.ModifierValues{IsElectric: true}) {
		t.Fatal("predicate matched road cycling")
	}
}

func TestNormalizeGearDefaultSports(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []string
		wantErr bool
	}{
		{name: "nil input", values: nil, want: []string{}},
		{name: "empty input", values: []string{}, want: []string{}},
		{name: "trims and dedupes", values: []string{" cycling ", "cycling", "running"}, want: []string{"cycling", "running"}},
		{name: "rejects tuple", values: []string{"cycling:mountain"}, wantErr: true},
		{name: "rejects unknown", values: []string{"unknown"}, wantErr: true},
		{name: "accepts broad fallback storage parent", values: []string{"fitness_equipment"}, want: []string{"fitness_equipment"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gear.NormalizeGearDefaultSports(tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizeGearDefaultSports() returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeGearDefaultSports() error = %v", err)
			}
			assertStringSlice(t, got, tt.want)
			if got == nil {
				t.Fatal("NormalizeGearDefaultSports() returned nil slice")
			}
		})
	}
}

func TestNormalizeManualGearDefaultSports(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []string
		wantErr bool
	}{
		{name: "nil input", values: nil, want: []string{}},
		{name: "empty input", values: []string{}, want: []string{}},
		{name: "trims and dedupes", values: []string{" cycling ", "cycling", "running"}, want: []string{"cycling", "running"}},
		{name: "rejects tuple", values: []string{"cycling:mountain"}, wantErr: true},
		{name: "rejects unknown", values: []string{"unknown"}, wantErr: true},
		{name: "rejects broad fallback storage parent", values: []string{"fitness_equipment"}, wantErr: true},
		{name: "accepts selectable top-level sport", values: []string{"cycling"}, want: []string{"cycling"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gear.NormalizeManualGearDefaultSports(tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizeManualGearDefaultSports() returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeManualGearDefaultSports() error = %v", err)
			}
			assertStringSlice(t, got, tt.want)
			if got == nil {
				t.Fatal("NormalizeManualGearDefaultSports() returned nil slice")
			}
		})
	}
}

// Verifies that NormalizeGearDefaultPredicates accepts only modifier keys
// declared in config/gear/applicable_modifiers.yaml (is_electric, is_indoor,
// is_virtual) and rejects others (is_commute).
func TestGearNormalizeDefaultsOnlyAppliesGearApplicableModifiers(t *testing.T) {
	t.Run("accepts is_electric", func(t *testing.T) {
		got, err := gear.NormalizeGearDefaultPredicates([]gear.GearDefaultPredicate{{
			SportType: "cycling",
			Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)},
		}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Modifiers == nil || got[0].Modifiers.IsElectric == nil || *got[0].Modifiers.IsElectric != true {
			t.Fatalf("got %#v, want is_electric=true predicate", got)
		}
	})
	t.Run("rejects is_commute", func(t *testing.T) {
		_, err := gear.NormalizeGearDefaultPredicates([]gear.GearDefaultPredicate{{
			SportType: "cycling",
			Modifiers: &sport.ModifierPatch{IsCommute: boolPtr(true)},
		}})
		if err == nil {
			t.Fatal("expected error for non-applicable is_commute modifier")
		}
	})
}

// Verifies the predicate match logic respects sport_type, sub_type, and
// modifier values together.
func TestGearMatchPredicateRespectsTypeKeyAndModifiers(t *testing.T) {
	mountain := "mountain"
	predicate := gear.GearDefaultPredicate{
		SportType: "cycling",
		SubType:   &mountain,
		Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)},
	}
	if !gear.GearDefaultPredicateMatchesActivity(predicate, "cycling", "mountain", sport.ModifierValues{IsElectric: true}) {
		t.Fatal("predicate failed to match electric mountain cycling")
	}
	if gear.GearDefaultPredicateMatchesActivity(predicate, "cycling", "mountain", sport.ModifierValues{IsElectric: false}) {
		t.Fatal("predicate matched non-electric mountain cycling")
	}
	if gear.GearDefaultPredicateMatchesActivity(predicate, "cycling", "road", sport.ModifierValues{IsElectric: true}) {
		t.Fatal("predicate matched road cycling instead of mountain")
	}
}

// Verifies that NormalizeGearDefaultPredicates dedupes by predicate key,
// distinguishing predicates that differ only in modifier values.
func TestGearCacheKeyDistinguishesByModifier(t *testing.T) {
	got, err := gear.NormalizeGearDefaultPredicates([]gear.GearDefaultPredicate{
		{SportType: "cycling", Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)}},
		{SportType: "cycling", Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(false)}},
		{SportType: "cycling", Modifiers: &sport.ModifierPatch{IsElectric: boolPtr(true)}}, // duplicate of first
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d predicates, want 2 (electric=true and electric=false distinct, duplicate dropped); got=%#v", len(got), got)
	}
}
