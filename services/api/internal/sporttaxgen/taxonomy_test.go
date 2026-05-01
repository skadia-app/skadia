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
