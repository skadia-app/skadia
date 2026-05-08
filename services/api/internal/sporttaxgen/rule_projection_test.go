package sporttaxgen

import (
	"os"
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
trait_conditions:
  - key: sport_environment
    labels: {de: Umgebung}
    field: sport_environment
    operators: [in]
    source: traits.environments
    option_labels:
      water: {de: Wasser}
      land: {de: Land}
  - key: sport_equipment_family
    labels: {de: Ausrüstungsart}
    field: sport_equipment_family
    operators: [in]
    source: traits.equipment_families
    option_labels:
      board: {de: Board}
  - key: sport_movement_family
    labels: {de: Bewegungsart}
    field: sport_movement_family
    operators: [in]
    source: traits.movement_families
    option_labels:
      paddle: {de: Paddeln}
`

func TestLoadRuleProjectionRejectsLocalModifierList(t *testing.T) {
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
	_, err := LoadRuleProjection(src, taxonomy, modifierRegistryForTest())
	if err == nil || !strings.Contains(err.Error(), "modifiers are derived") {
		t.Fatalf("LoadRuleProjection error = %v, want local modifier list error", err)
	}
}

func TestRuleProjectionModifierKeysMustMatchRegistry(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		t.Fatalf("open modifiers.yaml: %v", err)
	}
	defer file.Close()
	registry, err := LoadModifiers(file)
	if err != nil {
		t.Fatalf("LoadModifiers: %v", err)
	}
	projection, err := LoadRuleProjection(strings.NewReader(validRuleProjectionYAMLForTest), minimalRuleProjectionTaxonomy(t), modifierRegistryForTest())
	if err != nil {
		t.Fatalf("LoadRuleProjection: %v", err)
	}
	gotKeys := make([]string, 0, len(projection.Modifiers))
	gotLabels := map[string]string{}
	for _, modifier := range projection.Modifiers {
		gotKeys = append(gotKeys, modifier.Key)
		gotLabels[modifier.Key] = modifier.Labels["de"]
	}
	wantKeys := registry.ModifierKeysWithCapability("rule_condition")
	if !stringSlicesEqual(gotKeys, wantKeys) {
		t.Fatalf("rule projection modifier keys = %v, want %v", gotKeys, wantKeys)
	}
	for _, key := range wantKeys {
		def, _ := registry.ModifierByKey(key)
		if gotLabels[key] != def.Labels["de"] {
			t.Fatalf("rule projection label for %q = %q, want %q", key, gotLabels[key], def.Labels["de"])
		}
	}
}

func TestLoadRuleProjectionRejectsDuplicateSelectorKey(t *testing.T) {
	taxonomy := minimalRuleProjectionTaxonomy(t)
	src := strings.Replace(validRuleProjectionYAMLForTest, "key: action_set_sport_type", "key: condition_sport_tuple", 1)
	_, err := LoadRuleProjection(strings.NewReader(src), taxonomy, modifierRegistryForTest())
	if err == nil || !strings.Contains(err.Error(), "duplicate tuple selector key") {
		t.Fatalf("LoadRuleProjection error = %v, want duplicate selector key", err)
	}
}

func TestExpandRuleProjectionUsesSelectableTuplesAndModifierApplicability(t *testing.T) {
	taxonomy := minimalRuleProjectionTaxonomy(t)
	projection, err := LoadRuleProjection(strings.NewReader(validRuleProjectionYAMLForTest), taxonomy, modifierRegistryForTest())
	if err != nil {
		t.Fatalf("LoadRuleProjection: %v", err)
	}
	expanded, err := ExpandRuleProjection(taxonomy, projection, modifierRegistryForTest())
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
	if !expanded.HasTupleOption("unknown", "") {
		t.Fatalf("unknown must remain rule-addressable even though it is not a normal creation option")
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
	if got := expanded.TraitValueLabelDE("sport_environment", "water"); got != "Wasser" {
		t.Fatalf("sport_environment water label = %q, want Wasser", got)
	}
	if !expanded.HasTraitValue("sport_equipment_family", "board") {
		t.Fatal("sport_equipment_family should expose board")
	}
	if got := expanded.TraitValueLabelDE("sport_equipment_family", "board"); got != "Board" {
		t.Fatalf("sport_equipment_family board label = %q, want Board", got)
	}
	if !expanded.HasTraitValue("sport_movement_family", "paddle") {
		t.Fatal("sport_movement_family should expose paddle")
	}
	if got := expanded.TraitValueLabelDE("sport_movement_family", "paddle"); got != "Paddeln" {
		t.Fatalf("sport_movement_family paddle label = %q, want Paddeln", got)
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
				ModifierApplicability: modifierApplicability(false, true, true, true),
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
				ModifierApplicability: modifierApplicability(true, true, true, true),
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
				ModifierApplicability: modifierApplicability(false, true, false, false),
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
				ModifierApplicability: modifierApplicability(false, false, false, false),
			},
			{
				SportType:             "unknown",
				CategoryKey:           "fitness",
				Labels:                LocaleMap{"de": "Unbekannt"},
				AutoTitles:            LocaleMap{"de": "Training"},
				SegmentGroup:          "generic",
				Selectable:            boolPtr(false),
				Lifecycle:             "canonical",
				Traits:                Traits{Environments: []string{"generic"}, MovementFamilies: []string{"generic"}, EquipmentFamilies: []string{"generic"}, ActivityForm: "system", PropulsionModes: []string{"generic"}},
				ModifierApplicability: modifierApplicability(false, false, false, false),
			},
		},
	}
	applyTaxonomyDefaults(&taxonomy)
	return taxonomy
}

func modifierApplicability(isElectric, isIndoor, isVirtual, isCommute bool) *ModifierApplicability {
	applicability := ModifierApplicability{
		"is_electric": isElectric,
		"is_indoor":   isIndoor,
		"is_virtual":  isVirtual,
		"is_commute":  isCommute,
	}
	return &applicability
}
