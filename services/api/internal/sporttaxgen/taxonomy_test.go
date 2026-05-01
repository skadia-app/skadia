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
  - sport_type: generic
    category_key: other
    labels: {de: Allgemein}
    auto_titles: {de: Training}
    segment_group: generic
    typically_has_gps: false
    moving_threshold: 0
    is_indoor: false
    lifecycle: broad_fallback
    selectable: false
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
