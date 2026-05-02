package sporttaxgen

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLoadTaxonomyRejectsDuplicateSportTypes(t *testing.T) {
	src := `
categories:
  - key: run
    labels: {de: Läufe}
    color: {token: run, light_hex: "#EF4444"}
sports:
  - sport_type: running
    category_key: run
    labels: {de: Laufen}
    auto_titles: {de: Lauf}
    segment_group: foot
    typically_has_gps: true
    moving_threshold: 0.5
    is_indoor: false
    traits:
      environments: [land]
      movement_families: [run]
      equipment_families: []
      activity_form: single
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: true
      is_virtual: true
      is_commute: false
  - sport_type: running
    category_key: run
    labels: {de: Laufen 2}
    auto_titles: {de: Lauf}
    segment_group: foot
    typically_has_gps: true
    moving_threshold: 0.5
    is_indoor: false
    traits:
      environments: [land]
      movement_families: [run]
      equipment_families: []
      activity_form: single
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: true
      is_virtual: true
      is_commute: false
`
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted duplicate sport_type")
	}
}

func TestLoadTaxonomyRejectsDanglingCategory(t *testing.T) {
	src := `
categories:
  - key: run
    labels: {de: Läufe}
    color: {token: run, light_hex: "#EF4444"}
sports:
  - sport_type: cycling
    category_key: bike
    labels: {de: Radfahren}
    auto_titles: {de: Radfahrt}
    segment_group: bike
    typically_has_gps: true
    moving_threshold: 0.5
    is_indoor: false
    traits:
      environments: [land]
      movement_families: [pedal]
      equipment_families: [bike]
      activity_form: single
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: true
      is_indoor: false
      is_virtual: true
      is_commute: true
`
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted sport with missing category")
	}
}

func TestLoadTaxonomyAcceptsMinimalValidDocument(t *testing.T) {
	src := `
categories:
  - key: run
    labels: {de: Läufe}
    color: {token: run, light_hex: "#EF4444"}
sports:
  - sport_type: running
    category_key: run
    labels: {de: Laufen}
    auto_titles: {de: Lauf}
    segment_group: foot
    typically_has_gps: true
    moving_threshold: 0.5
    is_indoor: false
    traits:
      environments: [land]
      movement_families: [run]
      equipment_families: []
      activity_form: single
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: true
      is_virtual: true
      is_commute: true
    sub_types:
      - sub_type: trail
        labels: {de: Trail}
        details: {de: Trailrun}
        auto_titles: {de: Trailrun}
`
	taxonomy, err := LoadTaxonomy(strings.NewReader(src))
	if err != nil {
		t.Fatalf("LoadTaxonomy returned error: %v", err)
	}
	if len(taxonomy.Categories) != 1 || taxonomy.Categories[0].Key != "run" {
		t.Fatalf("categories = %#v, want one run category", taxonomy.Categories)
	}
	if len(taxonomy.Sports) != 1 || taxonomy.Sports[0].SportType != "running" {
		t.Fatalf("sports = %#v, want one running sport", taxonomy.Sports)
	}
}

func TestLoadTaxonomyAcceptsModifierDefaults(t *testing.T) {
	taxonomy, err := LoadTaxonomy(strings.NewReader(modifierDefaultsFixture()))
	if err != nil {
		t.Fatalf("LoadTaxonomy returned error: %v", err)
	}
	got := taxonomy.Sports[0].SubTypes[0].ModifierDefaults
	if got == nil || got.IsIndoor == nil || !*got.IsIndoor {
		t.Fatalf("subtype modifier defaults = %#v, want is_indoor true", got)
	}
}

func TestLoadTaxonomyRejectsModifierDefaultWhenApplicabilityFalse(t *testing.T) {
	src := strings.Replace(modifierDefaultsFixture(), "      is_indoor: true", "      is_indoor: false", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted modifier_defaults.is_indoor when modifier_applicability.is_indoor=false")
	}
}

func TestLoadTaxonomyRejectsEmptyModifierDefaults(t *testing.T) {
	src := strings.Replace(modifierDefaultsFixture(), "        modifier_defaults:\n          is_indoor: true", "        modifier_defaults: {}", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted empty modifier_defaults")
	}
}

func TestLoadTaxonomyRejectsUnknownModifierDefaultField(t *testing.T) {
	src := strings.Replace(modifierDefaultsFixture(), "        is_indoor: true", "        is_virtual: true", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted unknown modifier_defaults field")
	}
}

func TestLoadTaxonomyAcceptsLifecycleSelectableAndSubtypeThreshold(t *testing.T) {
	_, err := LoadTaxonomy(strings.NewReader(lifecycleFixture()))
	if err != nil {
		t.Fatalf("LoadTaxonomy returned error: %v", err)
	}
}

func TestLoadTaxonomyRejectsInvalidLifecycle(t *testing.T) {
	src := strings.Replace(lifecycleFixture(), "lifecycle: broad_fallback", "lifecycle: maybe", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted invalid lifecycle")
	}
}

func TestLoadTaxonomyPreservesLifecycleSelectableAndSubtypeThreshold(t *testing.T) {
	taxonomy, err := LoadTaxonomy(strings.NewReader(lifecycleFixture()))
	if err != nil {
		t.Fatalf("LoadTaxonomy returned error: %v", err)
	}
	if got := taxonomy.Sports[0].Lifecycle; got != "broad_fallback" {
		t.Fatalf("lifecycle = %q, want broad_fallback", got)
	}
	if taxonomy.Sports[0].Selectable == nil || *taxonomy.Sports[0].Selectable {
		t.Fatalf("selectable = %#v, want false", taxonomy.Sports[0].Selectable)
	}
	if taxonomy.Sports[0].SubTypes[0].MovingThreshold == nil || *taxonomy.Sports[0].SubTypes[0].MovingThreshold != 0.4 {
		t.Fatalf("subtype moving threshold = %#v, want 0.4", taxonomy.Sports[0].SubTypes[0].MovingThreshold)
	}
	if taxonomy.Sports[0].SubTypes[0].Selectable == nil || !*taxonomy.Sports[0].SubTypes[0].Selectable {
		t.Fatalf("subtype selectable = %#v, want true", taxonomy.Sports[0].SubTypes[0].Selectable)
	}
}

func lifecycleFixture() string {
	return `
categories:
  - key: other
    labels: {de: Sonstiges}
    color: {token: other, light_hex: "#6B7280"}
sports:
  - sport_type: paddling
    category_key: other
    labels: {de: Paddeln}
    auto_titles: {de: Paddeln}
    segment_group: generic
    typically_has_gps: false
    moving_threshold: 0
    is_indoor: false
    lifecycle: broad_fallback
    selectable: false
    traits:
      environments: [water]
      movement_families: [paddle]
      equipment_families: [paddle]
      activity_form: fallback
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: false
      is_virtual: false
      is_commute: false
    sub_types:
      - sub_type: mode
        labels: {de: Modus}
        details: {de: Modus}
        auto_titles: {de: Modus}
        moving_threshold: 0.4
        selectable: true
        traits:
          environments: [water]
          movement_families: [paddle]
          equipment_families: [paddle]
          activity_form: single
          propulsion_modes: [human]
`
}

func TestLoadTaxonomyRejectsDomainGroupKey(t *testing.T) {
	src := `
categories:
  - key: water
    labels: {de: Wassersport}
    color: {token: water, light_hex: "#3B82F6"}
sports:
  - sport_type: swimming
    category_key: water
    domain_group_key: water.swim
    labels: {de: Schwimmen}
    auto_titles: {de: Schwimmen}
    segment_group: swim
    typically_has_gps: false
    moving_threshold: 0.1
    is_indoor: false
    traits:
      environments: [water]
      movement_families: [swim]
      equipment_families: []
      activity_form: single
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: false
      is_virtual: false
      is_commute: false
`
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted domain_group_key")
	}
}

func TestLoadTaxonomyRejectsMissingTraits(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "    traits:\n      environments: [land]\n      movement_families: [run]\n      equipment_families: []\n      activity_form: single\n      propulsion_modes: [human]\n", "", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted sport without traits")
	}
}

func TestLoadTaxonomyRejectsInvalidTraitValue(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "movement_families: [run]", "movement_families: [jump]", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted invalid trait value")
	}
}

func TestLoadTaxonomyRejectsDuplicateTraitValue(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "environments: [land]", "environments: [land, land]", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted duplicate trait value")
	}
}

func TestLoadTaxonomyAllowsEmptyEquipmentFamilies(t *testing.T) {
	if _, err := LoadTaxonomy(strings.NewReader(validTraitFixture())); err != nil {
		t.Fatalf("LoadTaxonomy rejected empty equipment_families: %v", err)
	}
}

func TestLoadTaxonomyRequiresCompleteSubtypeTraits(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "          propulsion_modes: [human]\n", "", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted incomplete subtype traits")
	}
}

func TestLoadTaxonomyRejectsSelectableFallbackSport(t *testing.T) {
	src := strings.Replace(fallbackSportFixture("selectable: false"), "selectable: false", "selectable: true", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted selectable fallback sport")
	}
}

func TestLoadTaxonomyRejectsFallbackTraitsWithoutBroadFallbackLifecycle(t *testing.T) {
	src := strings.Replace(fallbackSportFixture("selectable: false"), "lifecycle: broad_fallback", "lifecycle: canonical", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted fallback activity_form without broad_fallback lifecycle")
	}
}

func TestLoadTaxonomyRejectsBroadFallbackLifecycleWithoutFallbackTraits(t *testing.T) {
	src := strings.Replace(fallbackSportFixture("selectable: false"), "activity_form: fallback", "activity_form: single", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted broad_fallback lifecycle without fallback traits")
	}
}

func TestLoadTaxonomyRejectsFallbackActivityFormOnSubtype(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "          activity_form: single", "          activity_form: fallback", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted fallback activity_form on subtype")
	}
}

func TestLoadTaxonomyRejectsFamilyActivityFormOnSubtype(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "          activity_form: single", "          activity_form: family", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted family activity_form on subtype")
	}
}

func TestLoadTaxonomyRejectsInheritedFallbackActivityFormOnSubtype(t *testing.T) {
	src := strings.Replace(fallbackSportFixture("selectable: false"), "        traits:\n          environments: [water]\n          movement_families: [paddle]\n          equipment_families: [paddle]\n          activity_form: single\n          propulsion_modes: [human]\n", "", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted inherited fallback activity_form on subtype")
	}
}

func TestLoadTaxonomyRejectsInheritedFamilyActivityFormOnSubtype(t *testing.T) {
	src := familySportFixture(false)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted inherited family activity_form on subtype")
	}
}

func TestLoadTaxonomyAllowsModeActivityFormOnSubtype(t *testing.T) {
	if _, err := LoadTaxonomy(strings.NewReader(flyingModeFixture("fly_vfr"))); err != nil {
		t.Fatalf("LoadTaxonomy rejected mode subtype traits: %v", err)
	}
}

func TestLoadTaxonomyRejectsModeActivityFormOnNormalSubtype(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "          activity_form: single", "          activity_form: mode", 1)
	src = strings.Replace(src, "          propulsion_modes: [human]", "          propulsion_modes: [generic]", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted mode activity_form on a normal subtype")
	}
}

func TestLoadTaxonomyRejectsModeActivityFormOnTopLevelSport(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "activity_form: single", "activity_form: mode", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted mode activity_form on top-level sport")
	}
}

func TestLoadTaxonomyRejectsGenericEnvironmentOnNonSystemSport(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "environments: [land]", "environments: [generic]", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted generic environment on non-system sport")
	}
}

func TestLoadTaxonomyRequiresGenericEnvironmentOnSystemSport(t *testing.T) {
	src := strings.Replace(systemSportFixture("generic"), "environments: [generic]", "environments: [land]", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted system sport without generic environment")
	}
}

func TestLoadTaxonomyRejectsSystemActivityFormOnNormalSport(t *testing.T) {
	src := strings.Replace(systemSportFixture("training"), "category_key: other", "category_key: other", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted system activity_form on a normal sport")
	}
}

func TestLoadTaxonomyRejectsGenericPropulsionOnSingleSport(t *testing.T) {
	src := strings.Replace(validTraitFixture(), "propulsion_modes: [human]", "propulsion_modes: [generic]", 1)
	_, err := LoadTaxonomy(strings.NewReader(src))
	if err == nil {
		t.Fatal("LoadTaxonomy accepted generic propulsion on single sport")
	}
}

func TestTraitVocabularyRejectsDeferredValues(t *testing.T) {
	for _, tc := range []struct {
		field string
		value string
	}{
		{field: "equipment_families", value: "snowshoe"},
		{field: "movement_families", value: "jump"},
		{field: "equipment_families", value: "shuttlecock"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			if containsString(TraitMovementFamilyValues, tc.value) ||
				containsString(TraitEquipmentFamilyValues, tc.value) ||
				containsString(TraitEnvironmentValues, tc.value) ||
				containsString(TraitPropulsionModeValues, tc.value) ||
				containsString(TraitActivityFormValues, tc.value) {
				t.Fatalf("deferred value %q is present in trait vocabulary", tc.value)
			}
			src := validTraitFixture()
			switch tc.field {
			case "movement_families":
				src = strings.Replace(src, "movement_families: [run]", "movement_families: ["+tc.value+"]", 1)
			case "equipment_families":
				src = strings.Replace(src, "equipment_families: []", "equipment_families: ["+tc.value+"]", 1)
			default:
				t.Fatalf("unhandled field %q", tc.field)
			}
			_, err := LoadTaxonomy(strings.NewReader(src))
			if err == nil {
				t.Fatalf("LoadTaxonomy accepted deferred value %q in %s", tc.value, tc.field)
			}
		})
	}
}

func TestLoadTaxonomyRequiresTraitsForEverySportInCurrentYAML(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()

	taxonomy, err := LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("LoadTaxonomy(sports.yaml): %v", err)
	}
	for _, sport := range taxonomy.Sports {
		if len(sport.Traits.Environments) == 0 ||
			len(sport.Traits.MovementFamilies) == 0 ||
			sport.Traits.ActivityForm == "" ||
			len(sport.Traits.PropulsionModes) == 0 {
			t.Fatalf("sport %q has incomplete traits: %#v", sport.SportType, sport.Traits)
		}
	}
}

func TestEffectiveSubTypeTraitsInheritsParentTraits(t *testing.T) {
	parent := Sport{Traits: Traits{
		Environments:      []string{"land"},
		MovementFamilies:  []string{"run"},
		EquipmentFamilies: []string{},
		ActivityForm:      "single",
		PropulsionModes:   []string{"human"},
	}}
	if got := EffectiveSubTypeTraits(parent, SubType{}); !stringSlicesEqual(got.Environments, parent.Traits.Environments) || got.ActivityForm != "single" {
		t.Fatalf("EffectiveSubTypeTraits() = %#v, want parent traits %#v", got, parent.Traits)
	}
}

func TestEffectiveSubTypeTraitsUsesSubtypeOverride(t *testing.T) {
	override := Traits{
		Environments:      []string{"indoor"},
		MovementFamilies:  []string{"run"},
		EquipmentFamilies: []string{"machine"},
		ActivityForm:      "single",
		PropulsionModes:   []string{"human"},
	}
	parent := Sport{Traits: Traits{
		Environments:      []string{"land"},
		MovementFamilies:  []string{"run"},
		EquipmentFamilies: []string{},
		ActivityForm:      "single",
		PropulsionModes:   []string{"human"},
	}}
	if got := EffectiveSubTypeTraits(parent, SubType{Traits: &override}); !stringSlicesEqual(got.Environments, []string{"indoor"}) || !stringSlicesEqual(got.EquipmentFamilies, []string{"machine"}) {
		t.Fatalf("EffectiveSubTypeTraits() = %#v, want subtype override %#v", got, override)
	}
}

func TestRepositorySportsYAMLLoads(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()

	taxonomy, err := LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("LoadTaxonomy(sports.yaml): %v", err)
	}
	if len(taxonomy.Categories) != 10 {
		t.Fatalf("category count = %d, want 10", len(taxonomy.Categories))
	}
	if len(taxonomy.Sports) < 60 {
		t.Fatalf("sport count = %d, want at least 60", len(taxonomy.Sports))
	}
}

func TestTaxonomySchemaMentionsValidatedFields(t *testing.T) {
	schema, err := os.ReadFile("../../../../config/sport-taxonomy/schema/taxonomy.schema.json")
	if err != nil {
		t.Fatalf("read taxonomy schema: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("taxonomy schema is not valid JSON: %v", err)
	}
	text := string(schema)
	for _, field := range []string{
		"categories",
		"sports",
		"sport_type",
		"category_key",
		"labels",
		"auto_titles",
		"segment_group",
		"typically_has_gps",
		"moving_threshold",
		"is_indoor",
		"lifecycle",
		"selectable",
		"modifier_applicability",
		"modifier_defaults",
		"traits",
		"environments",
		"movement_families",
		"equipment_families",
		"activity_form",
		"propulsion_modes",
		"sub_types",
		"sub_type",
		"details",
		"color",
		"token",
		"light_hex",
	} {
		if !strings.Contains(text, field) {
			t.Fatalf("taxonomy schema does not mention validated field %q", field)
		}
	}
}

func TestTaxonomySchemaSupportsOnlyIndoorModifierDefaults(t *testing.T) {
	schema, err := os.ReadFile("../../../../config/sport-taxonomy/schema/taxonomy.schema.json")
	if err != nil {
		t.Fatalf("read taxonomy schema: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("taxonomy schema is not valid JSON: %v", err)
	}

	defs := decoded["$defs"].(map[string]any)
	defaults := defs["modifierDefaults"].(map[string]any)
	if got := defaults["additionalProperties"]; got != false {
		t.Fatalf("modifierDefaults additionalProperties = %#v, want false", got)
	}
	properties := defaults["properties"].(map[string]any)
	isIndoor := properties["is_indoor"].(map[string]any)
	if got := isIndoor["type"]; got != "boolean" {
		t.Fatalf("modifierDefaults.is_indoor type = %#v, want boolean", got)
	}
	if _, ok := properties["is_virtual"]; ok {
		t.Fatal("modifierDefaults schema unexpectedly allows is_virtual")
	}

	sports := decoded["properties"].(map[string]any)["sports"].(map[string]any)
	sportItems := sports["items"].(map[string]any)
	sportProperties := sportItems["properties"].(map[string]any)
	assertSchemaRef(t, sportProperties, "modifier_defaults", "#/$defs/modifierDefaults")

	subTypes := sportProperties["sub_types"].(map[string]any)
	subTypeItems := subTypes["items"].(map[string]any)
	subTypeProperties := subTypeItems["properties"].(map[string]any)
	assertSchemaRef(t, subTypeProperties, "modifier_defaults", "#/$defs/modifierDefaults")
}

func TestTaxonomySchemaTraitEnumsMatchLoaderVocabulary(t *testing.T) {
	schema, err := os.ReadFile("../../../../config/sport-taxonomy/schema/taxonomy.schema.json")
	if err != nil {
		t.Fatalf("read taxonomy schema: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("taxonomy schema is not valid JSON: %v", err)
	}

	defs := decoded["$defs"].(map[string]any)
	traits := defs["traits"].(map[string]any)
	properties := traits["properties"].(map[string]any)

	assertSchemaEnum(t, properties, "environments", TraitEnvironmentValues)
	assertSchemaEnum(t, properties, "movement_families", TraitMovementFamilyValues)
	assertSchemaEnum(t, properties, "equipment_families", TraitEquipmentFamilyValues)
	assertSchemaEnum(t, properties, "activity_form", TraitActivityFormValues)
	assertSchemaEnum(t, properties, "propulsion_modes", TraitPropulsionModeValues)

	topLevelTraits := defs["topLevelTraits"].(map[string]any)
	allOf := topLevelTraits["allOf"].([]any)
	topLevelOverride := allOf[1].(map[string]any)
	topLevelProperties := topLevelOverride["properties"].(map[string]any)
	topLevelActivityForm := topLevelProperties["activity_form"].(map[string]any)
	topLevelEnumRaw := topLevelActivityForm["enum"].([]any)
	for _, value := range topLevelEnumRaw {
		if value.(string) == "mode" {
			t.Fatal("top-level trait schema allows activity_form mode")
		}
	}
}

func assertSchemaRef(t *testing.T, properties map[string]any, field, want string) {
	t.Helper()
	property := properties[field].(map[string]any)
	if got := property["$ref"]; got != want {
		t.Fatalf("schema property %s ref = %#v, want %q", field, got, want)
	}
}

func assertSchemaEnum(t *testing.T, properties map[string]any, field string, want []string) {
	t.Helper()
	property := properties[field].(map[string]any)
	var enumRaw []any
	if field == "activity_form" {
		enumRaw = property["enum"].([]any)
	} else {
		items := property["items"].(map[string]any)
		enumRaw = items["enum"].([]any)
	}
	got := make([]string, 0, len(enumRaw))
	for _, value := range enumRaw {
		got = append(got, value.(string))
	}
	if !stringSlicesEqual(got, want) {
		t.Fatalf("schema enum %s = %#v, want %#v", field, got, want)
	}
}

func modifierDefaultsFixture() string {
	return `
categories:
  - key: water
    labels: {de: Wasser}
    color: {token: water, light_hex: "#3B82F6"}
sports:
  - sport_type: diving
    category_key: water
    labels: {de: Tauchen}
    auto_titles: {de: Tauchgang}
    segment_group: swim
    typically_has_gps: false
    moving_threshold: 0.1
    is_indoor: false
    traits:
      environments: [water]
      movement_families: [swim]
      equipment_families: [breathing_gear]
      activity_form: family
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: true
      is_virtual: false
      is_commute: false
    sub_types:
      - sub_type: apnea_pool
        labels: {de: Apnoe Pool}
        details: {de: Apnoe-Tauchen (Pool)}
        auto_titles: {de: Apnoe-Tauchen}
        traits:
          environments: [water, indoor]
          movement_families: [swim]
          equipment_families: []
          activity_form: single
          propulsion_modes: [human]
        modifier_defaults:
          is_indoor: true
`
}

func validTraitFixture() string {
	return `
categories:
  - key: run
    labels: {de: Läufe}
    color: {token: run, light_hex: "#EF4444"}
sports:
  - sport_type: running
    category_key: run
    labels: {de: Laufen}
    auto_titles: {de: Lauf}
    segment_group: foot
    typically_has_gps: true
    moving_threshold: 0.5
    is_indoor: false
    traits:
      environments: [land]
      movement_families: [run]
      equipment_families: []
      activity_form: single
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: true
      is_virtual: true
      is_commute: true
    sub_types:
      - sub_type: trail
        labels: {de: Trail}
        details: {de: Trailrun}
        auto_titles: {de: Trailrun}
        traits:
          environments: [land]
          movement_families: [run]
          equipment_families: []
          activity_form: single
          propulsion_modes: [human]
`
}

func fallbackSportFixture(selectableLine string) string {
	return `
categories:
  - key: water
    labels: {de: Wasser}
    color: {token: water, light_hex: "#3B82F6"}
sports:
  - sport_type: paddling
    category_key: water
    labels: {de: Paddeln}
    auto_titles: {de: Paddeln}
    segment_group: paddle
    typically_has_gps: true
    moving_threshold: 0.1
    is_indoor: false
    lifecycle: broad_fallback
    ` + selectableLine + `
    traits:
      environments: [water]
      movement_families: [paddle]
      equipment_families: [paddle]
      activity_form: fallback
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: false
      is_virtual: false
      is_commute: false
    sub_types:
      - sub_type: generic_mode
        labels: {de: Modus}
        details: {de: Modus}
        auto_titles: {de: Modus}
        traits:
          environments: [water]
          movement_families: [paddle]
          equipment_families: [paddle]
          activity_form: single
          propulsion_modes: [human]
`
}

func familySportFixture(withOverride bool) string {
	subtypeTraits := ""
	if withOverride {
		subtypeTraits = `
        traits:
          environments: [land]
          movement_families: [strike]
          equipment_families: [racket, ball]
          activity_form: single
          propulsion_modes: [human]`
	}
	return `
categories:
  - key: game
    labels: {de: Spiel}
    color: {token: game, light_hex: "#F97316"}
sports:
  - sport_type: racket
    category_key: game
    labels: {de: Schlägersport}
    auto_titles: {de: Schlägersport}
    segment_group: generic
    typically_has_gps: false
    moving_threshold: 0.5
    is_indoor: false
    traits:
      environments: [land]
      movement_families: [strike]
      equipment_families: [racket, ball]
      activity_form: family
      propulsion_modes: [human]
    modifier_applicability:
      is_electric: false
      is_indoor: true
      is_virtual: false
      is_commute: false
    sub_types:
      - sub_type: squash
        labels: {de: Squash}
        details: {de: Squash}
        auto_titles: {de: Squash}` + subtypeTraits + `
`
}

func systemSportFixture(sportType string) string {
	return `
categories:
  - key: other
    labels: {de: Sonstiges}
    color: {token: other, light_hex: "#6B7280"}
sports:
  - sport_type: ` + sportType + `
    category_key: other
    labels: {de: System}
    auto_titles: {de: System}
    segment_group: generic
    typically_has_gps: false
    moving_threshold: 0
    is_indoor: false
    traits:
      environments: [generic]
      movement_families: [generic]
      equipment_families: [generic]
      activity_form: system
      propulsion_modes: [generic]
    modifier_applicability:
      is_electric: false
      is_indoor: false
      is_virtual: false
      is_commute: false
`
}

func flyingModeFixture(subType string) string {
	return `
categories:
  - key: outdoor
    labels: {de: Draußen}
    color: {token: outdoor, light_hex: "#10B981"}
sports:
  - sport_type: flying
    category_key: outdoor
    labels: {de: Fliegen}
    auto_titles: {de: Fliegen}
    segment_group: generic
    typically_has_gps: true
    moving_threshold: 0.5
    is_indoor: false
    traits:
      environments: [air]
      movement_families: [fly]
      equipment_families: [aircraft]
      activity_form: family
      propulsion_modes: [generic]
    modifier_applicability:
      is_electric: false
      is_indoor: false
      is_virtual: false
      is_commute: true
    sub_types:
      - sub_type: ` + subType + `
        labels: {de: Modus}
        details: {de: Modus}
        auto_titles: {de: Flug}
        traits:
          environments: [air]
          movement_families: [fly]
          equipment_families: [aircraft]
          activity_form: mode
          propulsion_modes: [generic]
`
}
