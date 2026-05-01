package sporttaxgen

import (
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
