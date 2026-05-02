package sporttaxgen

import (
	"os"
	"strings"
	"testing"
)

func TestLoadActivityFilterProjectionsRejectsDuplicatePresetKey(t *testing.T) {
	_, err := loadProjectionFixture(t, `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: bike
        label_ref: category:bike
        entries:
          - category_key: bike
`)
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted duplicate preset key")
	}
}

func TestLoadActivityFilterProjectionsRejectsDuplicateGroupKey(t *testing.T) {
	_, err := loadProjectionFixture(t, `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
      - key: run
        label_ref: category:bike
        entries:
          - category_key: bike
`)
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted duplicate group key")
	}
}

func TestLoadActivityFilterProjectionsRejectsDanglingCategory(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "category_key: run", "category_key: missing", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted dangling category")
	}
}

func TestLoadActivityFilterProjectionsRejectsDanglingSport(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "sport_type: pilates", "sport_type: missing", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted dangling sport")
	}
}

func TestLoadActivityFilterProjectionsRejectsDanglingSubtype(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "sub_type: pilates", "sub_type: missing", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted dangling subtype")
	}
}

func TestLoadActivityFilterProjectionsRejectsSubtypeWithoutSportType(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "sport_type: fitness_equipment\n            sub_type: pilates", "sub_type: pilates", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted subtype without sport type")
	}
}

func TestLoadActivityFilterProjectionsRejectsCategoryWithSubtype(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "category_key: run", "category_key: run\n            sub_type: trail", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted category with subtype")
	}
}

func TestLoadActivityFilterProjectionsRejectsUnknownField(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "labels: {de: Aktivitätsfilter}", "labels: {de: Aktivitätsfilter}\n    extra: nope", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted unknown field")
	}
}

func TestLoadActivityFilterProjectionsRejectsDomainGroupKey(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "sport_type: pilates", "domain_group_key: water.board\n          - sport_type: pilates", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted domain_group_key")
	}
}

func TestLoadActivityFilterProjectionsRejectsEntryWithMultipleTargets(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "category_key: run", "category_key: run\n            sport_type: running", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted entry with category and sport target")
	}
}

func TestLoadActivityFilterProjectionsRejectsEmptyEntry(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "category_key: run", "labels: {de: Leer}", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted empty target entry")
	}
}

func TestLoadActivityFilterProjectionsRejectsDanglingSportOrderEntry(t *testing.T) {
	_, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "category_key: run", "category_key: run\n            sport_order: [running, missing]", 1))
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted dangling sport_order entry")
	}
}

func TestExpandActivityFilterProjectionsRejectsDuplicateTupleInPreset(t *testing.T) {
	projection, err := loadProjectionFixture(t, `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: a
        labels: {de: A}
        entries:
          - sport_type: running
      - key: b
        labels: {de: B}
        entries:
          - sport_type: running
`)
	if err != nil {
		t.Fatalf("load projection fixture: %v", err)
	}
	_, err = ExpandActivityFilterPresets(testProjectionTaxonomy(), projection)
	if err == nil {
		t.Fatal("ExpandActivityFilterPresets accepted duplicate tuple in one preset")
	}
}

func TestExpandActivityFilterProjectionsRejectsDuplicateLeafLabelWithoutContext(t *testing.T) {
	projection, err := loadProjectionFixture(t, `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: duplicates
        labels: {de: Doppelt}
        entries:
          - sport_type: duplicate_one
          - sport_type: duplicate_two
`)
	if err != nil {
		t.Fatalf("load projection fixture: %v", err)
	}
	_, err = ExpandActivityFilterPresets(testProjectionTaxonomy(), projection)
	if err == nil {
		t.Fatal("ExpandActivityFilterPresets accepted duplicate parent labels")
	}
}

func TestExpandActivityFilterProjectionsAllowsParentContextForDuplicateLeafLabel(t *testing.T) {
	projection, err := loadProjectionFixture(t, validProjectionFixture())
	if err != nil {
		t.Fatalf("load projection fixture: %v", err)
	}
	expanded, err := ExpandActivityFilterPresets(testProjectionTaxonomy(), projection)
	if err != nil {
		t.Fatalf("ExpandActivityFilterPresets returned error: %v", err)
	}
	leaves := expanded[0].Groups[1].Leaves
	if leaves[0].Label["de"] != "Pilates" || leaves[1].Label["de"] != "Fitnessgerät: Pilates" {
		t.Fatalf("fitness labels = %#v, want top-level Pilates and Fitnessgerät: Pilates", leaves)
	}
}

func TestExpandActivityFilterProjectionsSuppressesPromotedParentLeafInCategoryExpansion(t *testing.T) {
	projection, err := loadProjectionFixture(t, `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: game
        label_ref: category:game
        entries:
          - category_key: game
`)
	if err != nil {
		t.Fatalf("load projection fixture: %v", err)
	}
	expanded, err := ExpandActivityFilterPresets(testProjectionTaxonomy(), projection)
	if err != nil {
		t.Fatalf("ExpandActivityFilterPresets returned error: %v", err)
	}
	keys := tupleKeys(expanded[0].Groups[0].Leaves)
	if contains(keys, "racket") {
		t.Fatalf("category expansion emitted promoted parent racket: %v", keys)
	}
	if !contains(keys, "racket:squash") || !contains(keys, "racket:badminton") {
		t.Fatalf("category expansion keys = %v, want racket subtypes", keys)
	}
}

func TestLoadCurrentActivityFilterProjection(t *testing.T) {
	taxonomyFile, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer taxonomyFile.Close()
	taxonomy, err := LoadTaxonomy(taxonomyFile)
	if err != nil {
		t.Fatalf("load sports.yaml: %v", err)
	}

	projectionFile, err := os.Open("../../../../config/sport-taxonomy/projections/activity_filters.yaml")
	if err != nil {
		t.Fatalf("open activity_filters.yaml: %v", err)
	}
	defer projectionFile.Close()
	projection, err := LoadActivityFilterProjections(projectionFile, taxonomy)
	if err != nil {
		t.Fatalf("LoadActivityFilterProjections returned error: %v", err)
	}
	if _, err := ExpandActivityFilterPresets(taxonomy, projection); err != nil {
		t.Fatalf("ExpandActivityFilterPresets returned error: %v", err)
	}
}

func TestProjectionSchemaMentionsValidatedFields(t *testing.T) {
	schema, err := os.ReadFile("../../../../config/sport-taxonomy/schema/projections.schema.json")
	if err != nil {
		t.Fatalf("read projections.schema.json: %v", err)
	}
	for _, field := range []string{"presets", "groups", "entries", "category_key", "sport_type", "sub_type", "sport_order", "labels", "label_ref", "label_mode"} {
		if !strings.Contains(string(schema), field) {
			t.Fatalf("schema does not mention %q", field)
		}
	}
}

func loadProjectionFixture(t *testing.T, src string) (ActivityFilterProjections, error) {
	t.Helper()
	return LoadActivityFilterProjections(strings.NewReader(src), testProjectionTaxonomy())
}

func validProjectionFixture() string {
	return `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
      - key: fitness
        labels: {de: Fitness}
        entries:
          - sport_type: pilates
          - sport_type: fitness_equipment
            sub_type: pilates
            label_mode: with_parent
`
}

func testProjectionTaxonomy() Taxonomy {
	return Taxonomy{
		Categories: []Category{
			{Key: "run", Labels: LocaleMap{"de": "Läufe"}, Color: Color{Token: "run", LightHex: "#EF4444"}},
			{Key: "bike", Labels: LocaleMap{"de": "Radfahrten"}, Color: Color{Token: "bike", LightHex: "#10B981"}},
			{Key: "fitness", Labels: LocaleMap{"de": "Fitness"}, Color: Color{Token: "fitness", LightHex: "#8B5CF6"}},
			{Key: "game", Labels: LocaleMap{"de": "Spiele"}, Color: Color{Token: "game", LightHex: "#F59E0B"}},
			{Key: "dup", Labels: LocaleMap{"de": "Doppelt"}, Color: Color{Token: "dup", LightHex: "#6B7280"}},
		},
		Sports: []Sport{
			testSport("running", "run", "Laufen", true, nil),
			testSport("cycling", "bike", "Radfahren", true, nil),
			testSport("pilates", "fitness", "Pilates", true, nil),
			testSport("fitness_equipment", "fitness", "Fitnessgerät", false, []SubType{
				testSubType("pilates", "Pilates", true),
			}),
			testSport("racket", "game", "Schlägersport", true, []SubType{
				testSubType("squash", "Squash", true),
				testSubType("badminton", "Badminton", true),
			}),
			testSport("duplicate_one", "dup", "Doppelt", true, nil),
			testSport("duplicate_two", "dup", "Doppelt", true, nil),
		},
	}
}

func testSport(sportType, categoryKey, label string, selectable bool, subTypes []SubType) Sport {
	return Sport{
		SportType:        sportType,
		CategoryKey:      categoryKey,
		Labels:           LocaleMap{"de": label},
		AutoTitles:       LocaleMap{"de": label},
		SegmentGroup:     "generic",
		Selectable:       &selectable,
		Lifecycle:        "canonical",
		SubTypes:         subTypes,
		Traits:           Traits{Environments: []string{"land"}, MovementFamilies: []string{"generic"}, ActivityForm: "single", PropulsionModes: []string{"human"}},
		ModifierDefaults: nil,
	}
}

func testSubType(subType, label string, selectable bool) SubType {
	return SubType{
		SubType:    subType,
		Labels:     LocaleMap{"de": label},
		Details:    LocaleMap{"de": label},
		AutoTitles: LocaleMap{"de": label},
		Selectable: &selectable,
	}
}

func tupleKeys(leaves []ExpandedActivityFilterLeaf) []string {
	keys := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		if leaf.SubType == "" {
			keys = append(keys, leaf.SportType)
		} else {
			keys = append(keys, leaf.SportType+":"+leaf.SubType)
		}
	}
	return keys
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
