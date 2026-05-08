package sport

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestGeneratedBackendTaxonomyIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	got, err := sporttaxgen.RenderGo(taxonomy)
	if err != nil {
		t.Fatalf("render Go taxonomy: %v", err)
	}
	want, err := os.ReadFile("taxonomy_generated.go")
	if err != nil {
		t.Fatalf("read taxonomy_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("taxonomy_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedBackendModifiersAreCurrent(t *testing.T) {
	modifiers := loadTestModifiers(t)
	got, err := sporttaxgen.RenderGoModifiers(modifiers)
	if err != nil {
		t.Fatalf("render Go modifiers: %v", err)
	}
	want, err := os.ReadFile("modifiers_generated.go")
	if err != nil {
		t.Fatalf("read modifiers_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("modifiers_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedModifierRegistryExposesMetadata(t *testing.T) {
	keys := SportModifierKeys()
	want := []string{"is_electric", "is_indoor", "is_virtual", "is_commute"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("SportModifierKeys() = %v, want %v", keys, want)
	}
	electric, ok := SportModifierByKey("is_electric")
	if !ok {
		t.Fatal("SportModifierByKey(is_electric) missing")
	}
	if electric.LabelDE != "E-Antrieb" || electric.ActionName != "set_electric" || electric.ActionParam != "electric" || electric.ShadowFSToken != "electric" {
		t.Fatalf("is_electric metadata = %#v", electric)
	}
	if got := SportModifierKeysWithCapability("modifier_filter"); strings.Join(got, ",") != "is_electric,is_indoor,is_virtual,is_commute" {
		t.Fatalf("modifier_filter keys = %v", got)
	}
	if got := SportModifierKeysWithCapability("taxonomy_default"); strings.Join(got, ",") != "is_indoor" {
		t.Fatalf("taxonomy_default keys = %v", got)
	}
}

func TestGeneratedBackendSegmentProjectionIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	modifiers := loadTestModifiers(t)
	activityFile, err := os.Open("../../../../config/sport-taxonomy/projections/activity_filters.yaml")
	if err != nil {
		t.Fatalf("open activity_filters.yaml: %v", err)
	}
	defer activityFile.Close()

	activityProjection, err := sporttaxgen.LoadActivityFilterProjections(activityFile, taxonomy)
	if err != nil {
		t.Fatalf("load activity filter projections: %v", err)
	}
	activityExpanded, err := sporttaxgen.ExpandActivityFilterPresets(taxonomy, activityProjection)
	if err != nil {
		t.Fatalf("expand activity filter projections: %v", err)
	}
	segmentFile, err := os.Open("../../../../config/sport-taxonomy/projections/segments.yaml")
	if err != nil {
		t.Fatalf("open segments.yaml: %v", err)
	}
	defer segmentFile.Close()
	segmentProjection, err := sporttaxgen.LoadSegmentProjection(segmentFile, taxonomy)
	if err != nil {
		t.Fatalf("load segment projection: %v", err)
	}
	segmentExpanded, err := sporttaxgen.ExpandSegmentProjection(taxonomy, segmentProjection)
	if err != nil {
		t.Fatalf("expand segment projection: %v", err)
	}
	ruleFile, err := os.Open("../../../../config/sport-taxonomy/projections/rules.yaml")
	if err != nil {
		t.Fatalf("open rules.yaml: %v", err)
	}
	defer ruleFile.Close()
	ruleProjection, err := sporttaxgen.LoadRuleProjection(ruleFile, taxonomy, modifiers)
	if err != nil {
		t.Fatalf("load rule projection: %v", err)
	}
	ruleExpanded, err := sporttaxgen.ExpandRuleProjection(taxonomy, ruleProjection, modifiers)
	if err != nil {
		t.Fatalf("expand rule projection: %v", err)
	}
	got, err := sporttaxgen.RenderGoProjections(activityExpanded, segmentExpanded, ruleExpanded)
	if err != nil {
		t.Fatalf("render Go projections: %v", err)
	}
	want, err := os.ReadFile("projections_generated.go")
	if err != nil {
		t.Fatalf("read projections_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("projections_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedFrontendActivityFilterProjectionIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	file, err := os.Open("../../../../config/sport-taxonomy/projections/activity_filters.yaml")
	if err != nil {
		t.Fatalf("open activity_filters.yaml: %v", err)
	}
	defer file.Close()
	projection, err := sporttaxgen.LoadActivityFilterProjections(file, taxonomy)
	if err != nil {
		t.Fatalf("load activity filter projections: %v", err)
	}
	expanded, err := sporttaxgen.ExpandActivityFilterPresets(taxonomy, projection)
	if err != nil {
		t.Fatalf("expand activity filter projections: %v", err)
	}
	got, err := sporttaxgen.RenderTSActivityFilterProjections(expanded)
	if err != nil {
		t.Fatalf("render TypeScript activity filter projection: %v", err)
	}
	want, err := os.ReadFile("../../../../frontend/src/lib/generatedSportFilterProjections.ts")
	if err != nil {
		t.Fatalf("read generatedSportFilterProjections.ts: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generatedSportFilterProjections.ts is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedFrontendSegmentProjectionIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	projection := loadTestSegmentProjection(t, taxonomy)
	expanded, err := sporttaxgen.ExpandSegmentProjection(taxonomy, projection)
	if err != nil {
		t.Fatalf("expand segment projection: %v", err)
	}
	got, err := sporttaxgen.RenderTSSegmentProjection(expanded)
	if err != nil {
		t.Fatalf("render TypeScript segment projection: %v", err)
	}
	want, err := os.ReadFile("../../../../frontend/src/lib/generatedSegmentProjections.ts")
	if err != nil {
		t.Fatalf("read generatedSegmentProjections.ts: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generatedSegmentProjections.ts is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedFrontendRuleProjectionIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	modifiers := loadTestModifiers(t)
	file, err := os.Open("../../../../config/sport-taxonomy/projections/rules.yaml")
	if err != nil {
		t.Fatalf("open rules.yaml: %v", err)
	}
	defer file.Close()
	projection, err := sporttaxgen.LoadRuleProjection(file, taxonomy, modifiers)
	if err != nil {
		t.Fatalf("load rule projection: %v", err)
	}
	expanded, err := sporttaxgen.ExpandRuleProjection(taxonomy, projection, modifiers)
	if err != nil {
		t.Fatalf("expand rule projection: %v", err)
	}
	got, err := sporttaxgen.RenderTSRuleProjection(expanded)
	if err != nil {
		t.Fatalf("render TypeScript rule projection: %v", err)
	}
	want, err := os.ReadFile("../../../../frontend/src/lib/generatedRuleProjections.ts")
	if err != nil {
		t.Fatalf("read generatedRuleProjections.ts: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generatedRuleProjections.ts is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedBackendSourceAliasesAreCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	modifiers := loadTestModifiers(t)
	sources := loadTestSourceAliases(t, taxonomy)
	gearDefaults, err := sporttaxgen.BuildGearDefaultProjection(taxonomy, sources, loadTestGearDefaultProjectionConfig(t, sources))
	if err != nil {
		t.Fatalf("build gear default projection: %v", err)
	}
	got, err := sporttaxgen.RenderGoSourceAliasesWithGearDefaults(sources, gearDefaults, modifiers, testSourceAliasIsApplicable)
	if err != nil {
		t.Fatalf("render Go source aliases: %v", err)
	}
	want, err := os.ReadFile("aliases_generated.go")
	if err != nil {
		t.Fatalf("read aliases_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("aliases_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedFrontendGearDefaultProjectionIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	sources := loadTestSourceAliases(t, taxonomy)
	gearDefaults, err := sporttaxgen.BuildGearDefaultProjection(taxonomy, sources, loadTestGearDefaultProjectionConfig(t, sources))
	if err != nil {
		t.Fatalf("build gear default projection: %v", err)
	}
	got, err := sporttaxgen.RenderTSGearDefaultProjection(gearDefaults)
	if err != nil {
		t.Fatalf("render TypeScript gear default projection: %v", err)
	}
	want, err := os.ReadFile("../../../../frontend/src/lib/generatedGearDefaultProjections.ts")
	if err != nil {
		t.Fatalf("read generatedGearDefaultProjections.ts: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generatedGearDefaultProjections.ts is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestGeneratedPredicateValidationSQLIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	segmentProjection := loadTestSegmentProjection(t, taxonomy)
	expandedSegmentProjection, err := sporttaxgen.ExpandSegmentProjection(taxonomy, segmentProjection)
	if err != nil {
		t.Fatalf("expand segment projection: %v", err)
	}
	got, err := sporttaxgen.RenderPredicateValidationSQL(taxonomy, expandedSegmentProjection)
	if err != nil {
		t.Fatalf("render predicate validation SQL: %v", err)
	}
	want, err := os.ReadFile("predicate_validation_generated.sql")
	if err != nil {
		t.Fatalf("read predicate_validation_generated.sql: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("predicate_validation_generated.sql is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestCategoryForSportIncludesGeneratedColor(t *testing.T) {
	cat := CategoryForSport("running")
	if cat.Color != "#EF4444" {
		t.Fatalf("running category color = %q, want #EF4444", cat.Color)
	}
}

// testSourceAliasIsApplicable is the applicability predicate for source aliases.
// The actual keys are owned by the dataimport subsystem; this copy exists here to
// avoid an import cycle (dataimport non-test imports sport).
// Update when dataimport/applicable_modifiers_generated.go changes.
func testSourceAliasIsApplicable(key string) bool {
	switch key {
	case "is_electric", "is_indoor", "is_virtual":
		return true
	}
	return false
}

func loadTestSourceAliases(t *testing.T, taxonomy sporttaxgen.Taxonomy) []sporttaxgen.SourceAliases {
	t.Helper()
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
			t.Fatalf("open alias file %s: %v", match, err)
		}
		opened = append(opened, file)
		stem := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))
		files[stem] = file
	}
	sources, err := sporttaxgen.LoadSourceAliasFiles(files, taxonomy, loadTestModifiers(t), testSourceAliasIsApplicable)
	if err != nil {
		t.Fatalf("load source aliases: %v", err)
	}
	return sources
}

func loadTestGearDefaultProjectionConfig(t *testing.T, sources []sporttaxgen.SourceAliases) sporttaxgen.GearDefaultProjectionConfig {
	t.Helper()
	file, err := os.Open("../../../../config/sport-taxonomy/projections/gear_defaults.yaml")
	if err != nil {
		t.Fatalf("open gear_defaults.yaml: %v", err)
	}
	defer file.Close()
	config, err := sporttaxgen.LoadGearDefaultProjectionConfig(file, sources)
	if err != nil {
		t.Fatalf("load gear_defaults.yaml: %v", err)
	}
	return config
}

func loadTestSegmentProjection(t *testing.T, taxonomy sporttaxgen.Taxonomy) sporttaxgen.SegmentProjection {
	t.Helper()
	file, err := os.Open("../../../../config/sport-taxonomy/projections/segments.yaml")
	if err != nil {
		t.Fatalf("open segments.yaml: %v", err)
	}
	defer file.Close()
	projection, err := sporttaxgen.LoadSegmentProjection(file, taxonomy)
	if err != nil {
		t.Fatalf("load segment projection: %v", err)
	}
	return projection
}

func loadTestTaxonomy(t *testing.T) sporttaxgen.Taxonomy {
	t.Helper()
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()

	taxonomy, err := sporttaxgen.LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("load taxonomy: %v", err)
	}
	return taxonomy
}

func loadTestModifiers(t *testing.T) sporttaxgen.ModifierRegistry {
	t.Helper()
	file, err := os.Open("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		t.Fatalf("open modifiers.yaml: %v", err)
	}
	defer file.Close()

	modifiers, err := sporttaxgen.LoadModifiers(file)
	if err != nil {
		t.Fatalf("load modifiers: %v", err)
	}
	return modifiers
}
