package sporttaxgen

import (
	"strings"
	"testing"
)

const validRuleProjectionYAMLForTest = `
tuple_selectors:
  - key: condition_sport_tuple
    labels: {de: Sportauswahl}
    field: sport_tuple
    operators: [in]
    source: selectable_sport_tuples
  - key: action_set_sport_type
    labels: {de: Sportart setzen}
    action: set_sport_type
    source: selectable_sport_tuples
modifiers:
  - key: is_electric
    labels: {de: E-Antrieb}
    condition_applicable_from: modifier_applicability.is_electric
    action_applicable_from: modifier_applicability.is_electric
    render_existing: true
  - key: is_indoor
    labels: {de: Indoor}
    condition_applicable_from: modifier_applicability.is_indoor
    action_applicable_from: modifier_applicability.is_indoor
    render_existing: true
  - key: is_virtual
    labels: {de: Virtuell}
    condition_applicable_from: modifier_applicability.is_virtual
    action_applicable_from: modifier_applicability.is_virtual
    render_existing: true
  - key: is_commute
    labels: {de: Pendeln}
    condition_applicable_from: modifier_applicability.is_commute
    action_applicable_from: modifier_applicability.is_commute
    render_existing: true
trait_conditions:
  - key: sport_environment
    labels: {de: Umgebung}
    field: sport_environment
    operators: [in]
    source: traits.environments
  - key: sport_equipment_family
    labels: {de: Ausrüstungsart}
    field: sport_equipment_family
    operators: [in]
    source: traits.equipment_families
  - key: sport_movement_family
    labels: {de: Bewegungsart}
    field: sport_movement_family
    operators: [in]
    source: traits.movement_families
`

func TestLoadRuleProjectionRejectsUnknownModifierSource(t *testing.T) {
	taxonomy := minimalRuleProjectionTaxonomy(t)
	src := strings.NewReader(`
tuple_selectors:
  - key: condition_sport_tuple
    labels: {de: Sportauswahl}
    field: sport_tuple
    operators: [in]
    source: selectable_sport_tuples
modifiers:
  - key: is_electric
    labels: {de: E-Antrieb}
    condition_applicable_from: modifier_applicability.nope
    action_applicable_from: modifier_applicability.is_electric
    render_existing: true
trait_conditions:
  - key: sport_environment
    labels: {de: Umgebung}
    field: sport_environment
    operators: [in]
    source: traits.environments
  - key: sport_equipment_family
    labels: {de: Ausrüstungsart}
    field: sport_equipment_family
    operators: [in]
    source: traits.equipment_families
  - key: sport_movement_family
    labels: {de: Bewegungsart}
    field: sport_movement_family
    operators: [in]
    source: traits.movement_families
`)
	_, err := LoadRuleProjection(src, taxonomy)
	if err == nil || !strings.Contains(err.Error(), "modifier_applicability.nope") {
		t.Fatalf("LoadRuleProjection error = %v, want unknown source error", err)
	}
}

func TestLoadRuleProjectionRejectsDuplicateSelectorKey(t *testing.T) {
	taxonomy := minimalRuleProjectionTaxonomy(t)
	src := strings.Replace(validRuleProjectionYAMLForTest, "key: action_set_sport_type", "key: condition_sport_tuple", 1)
	_, err := LoadRuleProjection(strings.NewReader(src), taxonomy)
	if err == nil || !strings.Contains(err.Error(), "duplicate tuple selector key") {
		t.Fatalf("LoadRuleProjection error = %v, want duplicate selector key", err)
	}
}

func TestExpandRuleProjectionUsesSelectableTuplesAndModifierApplicability(t *testing.T) {
	taxonomy := minimalRuleProjectionTaxonomy(t)
	projection, err := LoadRuleProjection(strings.NewReader(validRuleProjectionYAMLForTest), taxonomy)
	if err != nil {
		t.Fatalf("LoadRuleProjection: %v", err)
	}
	expanded, err := ExpandRuleProjection(taxonomy, projection)
	if err != nil {
		t.Fatalf("ExpandRuleProjection: %v", err)
	}
	if !expanded.HasTupleOption("cycling", "mountain") {
		t.Fatalf("expected cycling/mountain tuple option: %#v", expanded.TupleOptions)
	}
	if !expanded.HasTupleOption("cycling", "") {
		t.Fatalf("expected selectable parent cycling tuple option: %#v", expanded.TupleOptions)
	}
	if !expanded.HasTupleOption("running", "trail") {
		t.Fatalf("expected running/trail tuple option: %#v", expanded.TupleOptions)
	}
	if expanded.HasTupleOption("training", "") {
		t.Fatalf("non-selectable broad fallback parent must not be an action/condition tuple option")
	}
	if !expanded.ModifierApplicable("cycling", "mountain", "is_electric", "condition") {
		t.Fatal("cycling/mountain should expose is_electric condition control")
	}
	if expanded.ModifierApplicable("running", "trail", "is_electric", "condition") {
		t.Fatal("running/trail must not expose is_electric condition control")
	}
	if len(expanded.TraitConditions) != 3 {
		t.Fatalf("trait conditions len = %d, want 3", len(expanded.TraitConditions))
	}
	if !expanded.HasTraitValue("sport_environment", "water") {
		t.Fatal("sport_environment should expose water")
	}
	if !expanded.HasTraitValue("sport_equipment_family", "board") {
		t.Fatal("sport_equipment_family should expose board")
	}
	if !expanded.HasTraitValue("sport_movement_family", "paddle") {
		t.Fatal("sport_movement_family should expose paddle")
	}
}

func minimalRuleProjectionTaxonomy(t *testing.T) Taxonomy {
	t.Helper()
	boolPtr := func(v bool) *bool { return &v }
	taxonomy := Taxonomy{
		Categories: []Category{
			{Key: "run", Labels: LocaleMap{"de": "Läufe"}, Color: Color{Token: "run", LightHex: "#EF4444"}},
			{Key: "bike", Labels: LocaleMap{"de": "Radfahrten"}, Color: Color{Token: "bike", LightHex: "#10B981"}},
			{Key: "water", Labels: LocaleMap{"de": "Wasser"}, Color: Color{Token: "water", LightHex: "#3B82F6"}},
			{Key: "fitness", Labels: LocaleMap{"de": "Fitness"}, Color: Color{Token: "fitness", LightHex: "#8B5CF6"}},
		},
		Sports: []Sport{
			{
				SportType:             "running",
				CategoryKey:           "run",
				Labels:                LocaleMap{"de": "Laufen"},
				AutoTitles:            LocaleMap{"de": "Lauf"},
				SegmentGroup:          "foot",
				Selectable:            boolPtr(true),
				Lifecycle:             "canonical",
				Traits:                Traits{Environments: []string{"land"}, MovementFamilies: []string{"run"}, ActivityForm: "single", PropulsionModes: []string{"human"}},
				SubTypes:              []SubType{{SubType: "trail", Labels: LocaleMap{"de": "Trail"}, Details: LocaleMap{"de": "Trail"}, AutoTitles: LocaleMap{"de": "Trail"}, Selectable: boolPtr(true)}},
				ModifierApplicability: &ModifierApplicability{IsElectric: boolPtr(false), IsIndoor: boolPtr(true), IsVirtual: boolPtr(true), IsCommute: boolPtr(true)},
			},
			{
				SportType:             "cycling",
				CategoryKey:           "bike",
				Labels:                LocaleMap{"de": "Radfahren"},
				AutoTitles:            LocaleMap{"de": "Radfahrt"},
				SegmentGroup:          "bike",
				Selectable:            boolPtr(true),
				Lifecycle:             "canonical",
				Traits:                Traits{Environments: []string{"land"}, MovementFamilies: []string{"pedal"}, ActivityForm: "single", PropulsionModes: []string{"human"}},
				SubTypes:              []SubType{{SubType: "mountain", Labels: LocaleMap{"de": "MTB"}, Details: LocaleMap{"de": "MTB"}, AutoTitles: LocaleMap{"de": "MTB"}, Selectable: boolPtr(true)}},
				ModifierApplicability: &ModifierApplicability{IsElectric: boolPtr(true), IsIndoor: boolPtr(true), IsVirtual: boolPtr(true), IsCommute: boolPtr(true)},
			},
			{
				SportType:             "training",
				CategoryKey:           "fitness",
				Labels:                LocaleMap{"de": "Training"},
				AutoTitles:            LocaleMap{"de": "Training"},
				SegmentGroup:          "generic",
				Selectable:            boolPtr(false),
				Lifecycle:             "broad_fallback",
				Traits:                Traits{Environments: []string{"indoor"}, MovementFamilies: []string{"generic"}, ActivityForm: "fallback", PropulsionModes: []string{"human"}},
				ModifierApplicability: &ModifierApplicability{IsElectric: boolPtr(false), IsIndoor: boolPtr(true), IsVirtual: boolPtr(false), IsCommute: boolPtr(false)},
			},
			{
				SportType:             "stand_up_paddleboarding",
				CategoryKey:           "water",
				Labels:                LocaleMap{"de": "SUP"},
				AutoTitles:            LocaleMap{"de": "SUP"},
				SegmentGroup:          "paddle",
				Selectable:            boolPtr(true),
				Lifecycle:             "canonical",
				Traits:                Traits{Environments: []string{"water"}, MovementFamilies: []string{"paddle"}, EquipmentFamilies: []string{"board", "paddle"}, ActivityForm: "single", PropulsionModes: []string{"human"}},
				ModifierApplicability: &ModifierApplicability{IsElectric: boolPtr(false), IsIndoor: boolPtr(false), IsVirtual: boolPtr(false), IsCommute: boolPtr(false)},
			},
		},
	}
	applyTaxonomyDefaults(&taxonomy)
	return taxonomy
}
