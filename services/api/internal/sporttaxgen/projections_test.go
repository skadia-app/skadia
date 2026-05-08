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

func TestLoadActivityFilterProjectionsRejectsMissingPresets(t *testing.T) {
	_, err := loadProjectionFixture(t, `presets: []`)
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted empty presets")
	}
}

func TestLoadActivityFilterProjectionsRejectsMissingRequiredPreset(t *testing.T) {
	_, err := loadProjectionFixture(t, `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
`)
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted missing map_panel_gps preset")
	}
}

func TestLoadActivityFilterProjectionsRejectsPresetWithoutGroups(t *testing.T) {
	_, err := loadProjectionFixture(t, `
presets:
  - key: activity_all
    labels: {de: Aktivitätsfilter}
    groups: []
  - key: map_panel_gps
    labels: {de: Kartenfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
`)
	if err == nil {
		t.Fatal("LoadActivityFilterProjections accepted preset without groups")
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
  - key: map_panel_gps
    labels: {de: Kartenfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
            label_mode: with_parent
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
  - key: map_panel_gps
    labels: {de: Kartenfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
            label_mode: with_parent
`)
	if err != nil {
		t.Fatalf("load projection fixture: %v", err)
	}
	_, err = ExpandActivityFilterPresets(testProjectionTaxonomy(), projection)
	if err == nil {
		t.Fatal("ExpandActivityFilterPresets accepted duplicate parent labels")
	}
}

func TestExpandActivityFilterProjectionsRejectsDuplicateSubtypeLabelWithoutContext(t *testing.T) {
	projection, err := loadProjectionFixture(t, strings.Replace(validProjectionFixture(), "label_mode: with_parent", "label_mode: compact", 1))
	if err != nil {
		t.Fatalf("load projection fixture: %v", err)
	}
	_, err = ExpandActivityFilterPresets(testProjectionTaxonomy(), projection)
	if err == nil {
		t.Fatal("ExpandActivityFilterPresets accepted duplicate subtype labels without explicit context")
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
  - key: map_panel_gps
    labels: {de: Kartenfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
            label_mode: with_parent
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

func TestLoadSegmentProjectionRejectsDanglingSport(t *testing.T) {
	_, err := loadSegmentProjectionFixture(t, strings.Replace(validSegmentProjectionFixture(), "sport_type: running", "sport_type: missing", 1))
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted dangling sport")
	}
}

func TestLoadSegmentProjectionRejectsDanglingSegmentType(t *testing.T) {
	_, err := loadSegmentProjectionFixture(t, strings.Replace(validSegmentProjectionFixture(), "segment_types: [standard]", "segment_types: [missing]", 1))
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted dangling segment type")
	}
}

func TestLoadSegmentProjectionRejectsReservedActiveDomain(t *testing.T) {
	_, err := loadSegmentProjectionFixture(t, strings.Replace(validSegmentProjectionFixture(), "active_domains: [real_world_gps]", "active_domains: [virtual_route]", 1))
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted reserved active domain")
	}
}

func TestLoadSegmentProjectionRejectsDuplicateTupleForSegmentType(t *testing.T) {
	_, err := loadSegmentProjectionFixture(t, strings.Replace(validSegmentProjectionFixture(), "sport_type: running", "sport_type: running\n    segment_types: [standard]\n    domains: [real_world_gps]\n    sport_group: foot\n  - sport_type: running", 1))
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted duplicate tuple for same segment type/domain")
	}
}

func TestLoadSegmentProjectionRejectsTupleGroupDrift(t *testing.T) {
	_, err := loadSegmentProjectionFixture(t, strings.Replace(validSegmentProjectionFixture(), "segment_types: [standard]\n    domains: [real_world_gps]\n    sport_group: foot", "segment_types: [standard]\n    domains: [real_world_gps]\n    sport_group: foot\n  - sport_type: running\n    segment_types: [trail]\n    domains: [real_world_gps]\n    sport_group: generic", 1))
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted different sport_group values for one tuple")
	}
}

func TestLoadSegmentProjectionRejectsUnmarkedBroadFallback(t *testing.T) {
	src := strings.Replace(validSegmentProjectionFixture(), "sport_type: running", "sport_type: paddling", 1)
	src = strings.Replace(src, "sport_group: foot", "sport_group: paddle", 1)
	_, err := loadSegmentProjectionFixture(t, src)
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted unmarked broad fallback")
	}
}

func TestLoadSegmentProjectionAllowsMarkedLegacyFallbackCoverage(t *testing.T) {
	src := strings.Replace(validSegmentProjectionFixture(), "sport_type: running", "sport_type: paddling\n    eligibility_role: legacy_fallback_coverage", 1)
	src = strings.Replace(src, "sport_group: foot", "sport_group: paddle", 1)
	_, err := loadSegmentProjectionFixture(t, src)
	if err != nil {
		t.Fatalf("LoadSegmentProjection rejected marked legacy fallback coverage: %v", err)
	}
}

func TestLoadSegmentProjectionReportsFallbackGroupMismatchBeforeSelectable(t *testing.T) {
	src := strings.Replace(validSegmentProjectionFixture(), "sport_type: running", "sport_type: paddling\n    eligibility_role: legacy_fallback_coverage", 1)
	_, err := loadSegmentProjectionFixture(t, src)
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted mismatched legacy fallback coverage")
	}
	if !strings.Contains(err.Error(), "legacy_fallback_coverage requires matching broad fallback sport_group") {
		t.Fatalf("error = %v, want legacy fallback mismatch", err)
	}
}

func TestLoadCurrentSegmentProjection(t *testing.T) {
	taxonomyFile, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer taxonomyFile.Close()
	taxonomy, err := LoadTaxonomy(taxonomyFile)
	if err != nil {
		t.Fatalf("load sports.yaml: %v", err)
	}

	file, err := os.Open("../../../../config/sport-taxonomy/projections/segments.yaml")
	if err != nil {
		t.Fatalf("open segments.yaml: %v", err)
	}
	defer file.Close()
	projection, err := LoadSegmentProjection(file, taxonomy)
	if err != nil {
		t.Fatalf("LoadSegmentProjection returned error: %v", err)
	}
	expanded, err := ExpandSegmentProjection(taxonomy, projection)
	if err != nil {
		t.Fatalf("ExpandSegmentProjection returned error: %v", err)
	}
	if !expandedHasSegmentTuple(expanded, "wintersport", "snowshoeing", "") {
		t.Fatal("current segment projection does not expose snowshoeing for wintersport")
	}
	if expandedHasSegmentTuple(expanded, "wassersport", "diving", "") {
		t.Fatal("current segment projection unexpectedly exposes diving for wassersport")
	}
}

func TestLoadSegmentProjectionAcceptsRouteOptions(t *testing.T) {
	_, err := LoadSegmentProjection(strings.NewReader(strings.Replace(validSegmentProjectionFixture(), "eligible:", `
route_options:
  - sport_type: running
    promoted_sub_types: [indoor_running]

eligible:`, 1)), testProjectionTaxonomy())
	if err != nil {
		t.Fatalf("LoadSegmentProjection rejected route_options: %v", err)
	}
}

func TestLoadSegmentProjectionRejectsDanglingRouteOptionSubtype(t *testing.T) {
	_, err := LoadSegmentProjection(strings.NewReader(strings.Replace(validSegmentProjectionFixture(), "eligible:", `
route_options:
  - sport_type: running
    promoted_sub_types: [missing]

eligible:`, 1)), testProjectionTaxonomy())
	if err == nil {
		t.Fatal("LoadSegmentProjection accepted dangling route option subtype")
	}
}

func TestRenderTSSegmentProjectionEmitsRouteOptions(t *testing.T) {
	projection, err := LoadSegmentProjection(strings.NewReader(strings.Replace(validSegmentProjectionFixture(), "eligible:", `
route_options:
  - sport_type: running
    promoted_sub_types: [indoor_running]

eligible:`, 1)), testProjectionTaxonomy())
	if err != nil {
		t.Fatalf("LoadSegmentProjection() error = %v", err)
	}
	expanded, err := ExpandSegmentProjection(testProjectionTaxonomy(), projection)
	if err != nil {
		t.Fatalf("ExpandSegmentProjection() error = %v", err)
	}
	got, err := RenderTSSegmentProjection(expanded)
	if err != nil {
		t.Fatalf("RenderTSSegmentProjection() error = %v", err)
	}
	for _, want := range []string{
		"routeOptions: [",
		"{ sportType: 'running' }",
		"{ sportType: 'running', subType: 'indoor_running' }",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("generated TS segment projection missing %q:\n%s", want, got)
		}
	}
}

func loadProjectionFixture(t *testing.T, src string) (ActivityFilterProjections, error) {
	t.Helper()
	return LoadActivityFilterProjections(strings.NewReader(src), testProjectionTaxonomy())
}

func loadSegmentProjectionFixture(t *testing.T, src string) (SegmentProjection, error) {
	t.Helper()
	return LoadSegmentProjection(strings.NewReader(src), testProjectionTaxonomy())
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
            label_mode: with_parent
      - key: fitness
        labels: {de: Fitness}
        entries:
          - sport_type: pilates
          - sport_type: fitness_equipment
            sub_type: pilates
            label_mode: with_parent
  - key: map_panel_gps
    labels: {de: Kartenfilter}
    groups:
      - key: run
        label_ref: category:run
        entries:
          - category_key: run
            label_mode: with_parent
`
}

func validSegmentProjectionFixture() string {
	return `
domains:
  - key: real_world_gps
    labels: {de: GPS draußen}
    status: active
  - key: virtual_route
    labels: {de: Virtuelle Strecke}
    status: reserved
segment_types:
  - key: standard
    labels: {de: Standard}
    active_domains: [real_world_gps]
    compatibility_role: legacy_segment_type
  - key: trail
    labels: {de: Trail}
    active_domains: [real_world_gps]
    compatibility_role: legacy_segment_type
  - key: lift
    labels: {de: Lift}
    active_domains: []
    compatibility_role: legacy_lift
eligible:
  - sport_type: running
    segment_types: [standard]
    domains: [real_world_gps]
    sport_group: foot
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
			testSport("running", "run", "Laufen", true, []SubType{
				testSubType("indoor_running", "Indoor", true),
			}),
			testSport("walking", "run", "Gehen", true, []SubType{
				testSubType("indoor_walking", "Indoor", true),
			}),
			testSport("cycling", "bike", "Radfahren", true, nil),
			testBroadFallbackSport("paddling", "water", "Paddeln", "paddle"),
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

func testBroadFallbackSport(sportType, categoryKey, label, segmentGroup string) Sport {
	selectable := false
	sport := testSport(sportType, categoryKey, label, false, nil)
	sport.Selectable = &selectable
	sport.SegmentGroup = segmentGroup
	sport.Lifecycle = "broad_fallback"
	sport.Traits.ActivityForm = "fallback"
	return sport
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

func expandedHasSegmentTuple(projection ExpandedSegmentProjection, segmentType, sportType, subType string) bool {
	for _, entry := range projection.Entries {
		if entry.SegmentType == segmentType && entry.SportType == sportType && entry.SubType == subType {
			return true
		}
	}
	return false
}
