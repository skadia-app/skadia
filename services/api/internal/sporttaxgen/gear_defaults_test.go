package sporttaxgen

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadGearDefaultProjectionConfigLoadsRepositoryFixture loads the real
// gear_defaults.yaml from the repository and verifies the loader accepts it
// without error. Moved here from aliases_test.go (same package, no logic change).
func TestLoadGearDefaultProjectionConfigLoadsRepositoryFixture(t *testing.T) {
	taxonomyFile, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer taxonomyFile.Close()
	taxonomy, err := LoadTaxonomy(taxonomyFile)
	if err != nil {
		t.Fatalf("load sports.yaml: %v", err)
	}

	matches, err := filepath.Glob("../../../../config/sport-taxonomy/aliases/*.yaml")
	if err != nil {
		t.Fatalf("glob aliases: %v", err)
	}
	files := make(map[string]io.Reader, len(matches))
	opened := make([]*os.File, 0, len(matches))
	defer func() {
		for _, file := range opened {
			_ = file.Close()
		}
	}()
	for _, match := range matches {
		file, err := os.Open(match)
		if err != nil {
			t.Fatalf("open aliases %s: %v", match, err)
		}
		opened = append(opened, file)
		stem := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))
		files[stem] = file
	}
	sources, err := LoadSourceAliasFiles(files, taxonomy, modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err != nil {
		t.Fatalf("LoadSourceAliasFiles() error = %v", err)
	}
	configFile, err := os.Open("../../../../config/sport-taxonomy/projections/gear_defaults.yaml")
	if err != nil {
		t.Fatalf("open gear_defaults.yaml: %v", err)
	}
	defer configFile.Close()
	config, err := LoadGearDefaultProjectionConfig(configFile, sources)
	if err != nil {
		t.Fatalf("LoadGearDefaultProjectionConfig() error = %v", err)
	}
	if !stringSlicesEqual(config.ProjectionSources, []string{"strava"}) {
		t.Fatalf("projection_sources = %v, want [strava]", config.ProjectionSources)
	}
	projection, err := BuildGearDefaultProjection(taxonomy, sources, config)
	if err != nil {
		t.Fatalf("BuildGearDefaultProjection() error = %v", err)
	}
	if len(projection.Sources) != 1 || projection.Sources[0].Source != "strava" {
		t.Fatalf("projected sources = %#v, want only strava", projection.Sources)
	}
}

// TestLoadAcceptsApplicableSportPredicatesBlock verifies the loader accepts a
// YAML with the new applicable_sport_predicates key and populates the struct.
func TestLoadAcceptsApplicableSportPredicatesBlock(t *testing.T) {
	src := `
projection_sources: [strava]
applicable_sport_predicates:
  - gear_type_key: bike
    predicates:
      - sport_type: cycling
`
	config, err := LoadGearDefaultProjectionConfig(strings.NewReader(src), []SourceAliases{{Source: "strava"}})
	if err != nil {
		t.Fatalf("LoadGearDefaultProjectionConfig() error = %v", err)
	}
	if len(config.ApplicableSportPredicates) != 1 {
		t.Fatalf("ApplicableSportPredicates len = %d, want 1", len(config.ApplicableSportPredicates))
	}
	entry := config.ApplicableSportPredicates[0]
	if entry.GearTypeKey != "bike" {
		t.Fatalf("GearTypeKey = %q, want %q", entry.GearTypeKey, "bike")
	}
	if len(entry.Predicates) != 1 {
		t.Fatalf("Predicates len = %d, want 1", len(entry.Predicates))
	}
	if entry.Predicates[0].SportType != "cycling" {
		t.Fatalf("Predicates[0].SportType = %q, want %q", entry.Predicates[0].SportType, "cycling")
	}
	if entry.Predicates[0].SubType != nil {
		t.Fatalf("Predicates[0].SubType = %v, want nil", entry.Predicates[0].SubType)
	}
}

// TestSchemaRejectsUnknownPredicateField verifies that KnownFields(true)
// causes the loader to reject an unknown field inside a predicate entry.
func TestSchemaRejectsUnknownPredicateField(t *testing.T) {
	src := `
projection_sources: [strava]
applicable_sport_predicates:
  - gear_type_key: bike
    predicates:
      - sport_type: cycling
        garbage: x
`
	_, err := LoadGearDefaultProjectionConfig(strings.NewReader(src), []SourceAliases{{Source: "strava"}})
	if err == nil {
		t.Fatal("LoadGearDefaultProjectionConfig accepted unknown predicate field, want error")
	}
}
