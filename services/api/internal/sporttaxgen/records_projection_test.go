package sporttaxgen

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLoadRecordsProjectionLoadsRepositoryFixture(t *testing.T) {
	taxonomy := loadRecordsTestTaxonomy(t)
	file, err := os.Open("../../../../config/sport-taxonomy/projections/records.yaml")
	if err != nil {
		t.Fatalf("open records.yaml: %v", err)
	}
	defer file.Close()

	projection, err := LoadRecordsProjection(file, taxonomy)
	if err != nil {
		t.Fatalf("LoadRecordsProjection returned error: %v", err)
	}
	if len(projection.RecordGroups) != 10 {
		t.Fatalf("record group count = %d, want 10", len(projection.RecordGroups))
	}
}

func TestLoadRecordsProjectionRejectsInvalidReferences(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "invalid sport", src: strings.Replace(validRecordsProjectionFixture(), "sport_type: running", "sport_type: missing", 1)},
		{name: "invalid subtype", src: strings.Replace(validRecordsProjectionFixture(), "sub_types: [trail]", "sub_types: [missing]", 1)},
		{name: "duplicate record group key", src: strings.Replace(validRecordsProjectionFixture(), "key: running_trail", "key: running", 1)},
		{name: "missing category profile", src: strings.Replace(validRecordsProjectionFixture(), "category_profile: running_full", "category_profile: missing", 1)},
		{name: "missing category set reference", src: strings.Replace(validRecordsProjectionFixture(), "category_sets: [distances_running, superlatives_basic]", "category_sets: [distances_running, missing]", 1)},
		{name: "missing main category", src: strings.Replace(validRecordsProjectionFixture(), "main_category: laufen", "main_category: missing", 1)},
		{name: "group omitted from main category", src: strings.Replace(validRecordsProjectionFixture(), "record_groups: [running, running_trail]", "record_groups: [running]", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadRecordsProjection(strings.NewReader(tc.src), recordsProjectionTestTaxonomy())
			if err == nil {
				t.Fatalf("LoadRecordsProjection accepted %s", tc.name)
			}
		})
	}
}

func TestLoadRecordsProjectionRejectsEmptyRequiredCollectionsAndLabels(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "empty record groups", src: strings.Replace(validRecordsProjectionFixture(), "record_groups:", "record_groups: []\nmain_categories_disabled:", 1)},
		{name: "group missing label", src: strings.Replace(validRecordsProjectionFixture(), "labels: {de: Laufen}", "labels: {de: \"\"}", 1)},
		{name: "group missing color", src: strings.Replace(validRecordsProjectionFixture(), "color: \"#3B82F6\"", "color: \"\"", 1)},
		{name: "group missing match rules", src: strings.Replace(validRecordsProjectionFixture(), "match:\n      - priority: 10", "match: []\n    disabled_match:\n      - priority: 10", 1)},
		{name: "main category missing label", src: strings.Replace(validRecordsProjectionFixture(), "labels: {de: Laufen}", "labels: {de: \"\"}", 3)},
		{name: "main category missing color", src: strings.Replace(validRecordsProjectionFixture(), "color: \"#3B82F6\"", "color: \"\"", 3)},
		{name: "main category missing groups", src: strings.Replace(validRecordsProjectionFixture(), "record_groups: [running, running_trail]", "record_groups: []", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadRecordsProjection(strings.NewReader(tc.src), recordsProjectionTestTaxonomy())
			if err == nil {
				t.Fatalf("LoadRecordsProjection accepted %s", tc.name)
			}
		})
	}
}

func TestLoadRecordsProjectionRejectsInvalidColorHex(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "record group invalid color", src: strings.Replace(validRecordsProjectionFixture(), "color: \"#3B82F6\"", "color: \"blue\"", 1)},
		{name: "main category invalid color", src: strings.Replace(validRecordsProjectionFixture(), "color: \"#3B82F6\"", "color: \"blue\"", 3)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadRecordsProjection(strings.NewReader(tc.src), recordsProjectionTestTaxonomy())
			if err == nil {
				t.Fatalf("LoadRecordsProjection accepted %s", tc.name)
			}
		})
	}
}

func TestLoadRecordsProjectionRejectsInvalidCategoryDefinitions(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "duplicate expanded category key", src: strings.Replace(validRecordsProjectionFixture(), "key: sup_longest_distance", "key: dist_1km", 1)},
		{name: "value higher without duration", src: strings.Replace(validRecordsProjectionFixture(), "duration_s: 5", "", 1)},
		{name: "value higher non-positive duration", src: strings.Replace(validRecordsProjectionFixture(), "duration_s: 5", "duration_s: 0", 1)},
		{name: "time lower with two metrics", src: strings.Replace(validRecordsProjectionFixture(), "distance_m: 1000", "distance_m: 1000, elevation_m: 10", 1)},
		{name: "time lower non-positive metric", src: strings.Replace(validRecordsProjectionFixture(), "distance_m: 1000", "distance_m: 0", 1)},
		{name: "superlative with metric", src: strings.Replace(validRecordsProjectionFixture(), "type: value_superlative", "type: value_superlative, distance_m: 1", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadRecordsProjection(strings.NewReader(tc.src), recordsProjectionTestTaxonomy())
			if err == nil {
				t.Fatalf("LoadRecordsProjection accepted %s", tc.name)
			}
		})
	}
}

func TestLoadRecordsProjectionRejectsInvalidSubtypeSentinelUse(t *testing.T) {
	src := strings.Replace(validRecordsProjectionFixture(), "exclude_sub_types: [treadmill]", "exclude_sub_types: [\"\"]", 1)
	_, err := LoadRecordsProjection(strings.NewReader(src), recordsProjectionTestTaxonomy())
	if err == nil {
		t.Fatal("LoadRecordsProjection accepted empty subtype sentinel in exclude_sub_types")
	}
}

func TestExpandRecordsProjectionSortsRulesAndExpandsCategories(t *testing.T) {
	projection, err := LoadRecordsProjection(strings.NewReader(validRecordsProjectionFixture()), recordsProjectionTestTaxonomy())
	if err != nil {
		t.Fatalf("LoadRecordsProjection returned error: %v", err)
	}
	expanded, err := ExpandRecordsProjection(recordsProjectionTestTaxonomy(), projection)
	if err != nil {
		t.Fatalf("ExpandRecordsProjection returned error: %v", err)
	}

	if got, want := len(expanded.Groups), 2; got != want {
		t.Fatalf("expanded group count = %d, want %d", got, want)
	}
	gotCategoryKeys := []string{}
	for _, category := range expanded.Groups[0].Categories {
		gotCategoryKeys = append(gotCategoryKeys, category.Key)
	}
	wantCategoryKeys := []string{"dist_1km", "power_5s", "sup_longest_distance"}
	if !reflect.DeepEqual(gotCategoryKeys, wantCategoryKeys) {
		t.Fatalf("expanded category keys = %#v, want %#v", gotCategoryKeys, wantCategoryKeys)
	}

	gotRules := []string{}
	for _, rule := range expanded.MatchRules {
		gotRules = append(gotRules, rule.RecordGroup)
	}
	wantRules := []string{"running_trail", "running"}
	if !reflect.DeepEqual(gotRules, wantRules) {
		t.Fatalf("expanded match rule order = %#v, want %#v", gotRules, wantRules)
	}
}

func TestRenderRecordsProjectionOutputsGoAndTypeScriptShapes(t *testing.T) {
	projection, err := LoadRecordsProjection(strings.NewReader(validRecordsProjectionFixture()), recordsProjectionTestTaxonomy())
	if err != nil {
		t.Fatalf("LoadRecordsProjection returned error: %v", err)
	}
	expanded, err := ExpandRecordsProjection(recordsProjectionTestTaxonomy(), projection)
	if err != nil {
		t.Fatalf("ExpandRecordsProjection returned error: %v", err)
	}

	goOut, err := RenderGoRecordsProjection(expanded)
	if err != nil {
		t.Fatalf("RenderGoRecordsProjection returned error: %v", err)
	}
	goText := string(goOut)
	for _, want := range []string{
		"package bestefforts",
		"var generatedRecordGroups = []RecordGroup",
		"TypeTimeLower",
		"recordBoolPtr(false)",
		"SubTypes: []string{}",
	} {
		if !strings.Contains(goText, want) {
			t.Fatalf("generated Go missing %q:\n%s", want, goText)
		}
	}

	tsOut, err := RenderTSRecordsProjection(expanded)
	if err != nil {
		t.Fatalf("RenderTSRecordsProjection returned error: %v", err)
	}
	tsText := string(tsOut)
	for _, want := range []string{
		"export const GENERATED_RECORD_GROUPS",
		"export const GENERATED_RECORD_MAIN_CATEGORIES",
		"export const GENERATED_RECORD_MATCH_RULES",
		"export const GENERATED_RECORD_CATEGORY_ORDER",
		"export const GENERATED_RECORD_POWER_CATEGORY_ORDER",
		"'dist_1km': '1 km'",
		"subTypes: []",
		"excludeSubTypes: ['treadmill']",
		"modifiers: { isIndoor: false }",
		"durationS: 5",
	} {
		if !strings.Contains(tsText, want) {
			t.Fatalf("generated TypeScript missing %q:\n%s", want, tsText)
		}
	}
}

func TestRenderGoRecordsProjectionRejectsUnknownCategoryType(t *testing.T) {
	_, err := RenderGoRecordsProjection(ExpandedRecordsProjection{
		Groups: []ExpandedRecordGroup{
			{
				Key: "running",
				Label: LocaleMap{
					"de": "Laufen",
				},
				Categories: []RecordCategory{
					{Key: "bad", Type: "future_type", Labels: LocaleMap{"de": "Bad"}},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "future_type") {
		t.Fatalf("error = %v, want unknown category type rejection", err)
	}
}

func loadRecordsTestTaxonomy(t *testing.T) Taxonomy {
	t.Helper()
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()
	taxonomy, err := LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("load taxonomy: %v", err)
	}
	return taxonomy
}

func validRecordsProjectionFixture() string {
	return `
category_sets:
  - key: distances_running
    categories:
      - {key: dist_1km, type: time_lower, distance_m: 1000, labels: {de: 1 km}}
  - key: power
    categories:
      - {key: power_5s, type: value_higher, duration_s: 5, labels: {de: 5 Sek}}
  - key: superlatives_basic
    categories:
      - {key: sup_longest_distance, type: value_superlative, labels: {de: Längste Distanz}}

category_profiles:
  - key: running_full
    category_sets: [distances_running, superlatives_basic]
  - key: running_power
    category_sets: [distances_running, power, superlatives_basic]

record_groups:
  - key: running
    labels: {de: Laufen}
    main_category: laufen
    color: "#3B82F6"
    category_profile: running_power
    match:
      - priority: 10
        sport_type: running
        exclude_sub_types: [treadmill]
        modifiers: {is_indoor: false}
  - key: running_trail
    labels: {de: Laufen Trail}
    main_category: laufen
    color: "#3B82F6"
    category_profile: running_full
    match:
      - priority: 20
        sport_type: running
        sub_types: [trail]

main_categories:
  - key: laufen
    labels: {de: Laufen}
    color: "#3B82F6"
    record_groups: [running, running_trail]
    sub_filters:
      - {label: Alle, record_groups: [running, running_trail]}
    power_curve: {label: Laufen, record_groups: [running]}
`
}

func recordsProjectionTestTaxonomy() Taxonomy {
	taxonomy := testProjectionTaxonomy()
	taxonomy.Sports[0].SubTypes = append(taxonomy.Sports[0].SubTypes, testSubType("trail", "Trail", true), testSubType("treadmill", "Laufband", true))
	return taxonomy
}
