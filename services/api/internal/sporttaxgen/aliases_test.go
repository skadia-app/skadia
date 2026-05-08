package sporttaxgen

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func modifierRegistryForTest() ModifierRegistry {
	registry, err := LoadModifiers(strings.NewReader(validModifierRegistryFixture()))
	if err != nil {
		panic(err)
	}
	return registry
}

// sourceAliasPredicateForTest builds an isApplicable predicate from the registry's
// source_alias capability. Tests use the full registry capability so they stay
// consistent with the modifiers.yaml fixture without hard-coding key names.
func sourceAliasPredicateForTest() func(string) bool {
	registry := modifierRegistryForTest()
	return func(key string) bool {
		return registry.HasCapability(key, "source_alias")
	}
}

func TestLoadSourceAliasesAcceptsModifierPatches(t *testing.T) {
	source, err := LoadSourceAliases(strings.NewReader(`
source: strava
aliases:
- raw: Virtual Ride
  sport_type: cycling
  modifiers: {is_virtual: true}
- raw: E-Bike Ride
  sport_type: cycling
  modifiers: {is_electric: true}
- raw: Treadmill
  sport_type: running
  sub_type: treadmill
  modifiers: {is_indoor: true}
`), "strava", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err != nil {
		t.Fatalf("LoadSourceAliases() error = %v", err)
	}
	if got := source.Aliases[0].Modifiers["is_virtual"]; got == nil || !*got {
		t.Fatalf("is_virtual patch = %#v, want true", got)
	}
}

func TestLoadSourceAliasesRejectsSourceFilenameMismatch(t *testing.T) {
	_, err := LoadSourceAliases(strings.NewReader(`
source: strava
aliases:
- raw: Ride
  sport_type: cycling
`), "gpx", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err == nil {
		t.Fatal("LoadSourceAliases accepted source filename mismatch")
	}
}

func TestLoadSourceAliasesRejectsDuplicateRawWithinSource(t *testing.T) {
	_, err := LoadSourceAliases(strings.NewReader(`
source: gpx
aliases:
- raw: ride
  sport_type: cycling
- raw: ride
  sport_type: running
`), "gpx", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err == nil {
		t.Fatal("LoadSourceAliases accepted duplicate raw")
	}
}

func TestLoadSourceAliasesRejectsDuplicateRawAfterTrim(t *testing.T) {
	_, err := LoadSourceAliases(strings.NewReader(`
source: gpx
aliases:
- raw: " ride "
  sport_type: cycling
- raw: ride
  sport_type: running
`), "gpx", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err == nil {
		t.Fatal("LoadSourceAliases accepted duplicate raw after trim")
	}
}

func TestLoadSourceAliasesRejectsDanglingSportType(t *testing.T) {
	_, err := LoadSourceAliases(strings.NewReader(`
source: gpx
aliases:
- raw: ride
  sport_type: does_not_exist
`), "gpx", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err == nil {
		t.Fatal("LoadSourceAliases accepted dangling sport_type")
	}
}

func TestLoadSourceAliasesRejectsDanglingSubType(t *testing.T) {
	_, err := LoadSourceAliases(strings.NewReader(`
source: gpx
aliases:
- raw: ride
  sport_type: cycling
  sub_type: does_not_exist
`), "gpx", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err == nil {
		t.Fatal("LoadSourceAliases accepted dangling sub_type")
	}
}

func TestLoadSourceAliasesRejectsCommuteModifier(t *testing.T) {
	_, err := LoadSourceAliases(strings.NewReader(`
source: strava
aliases:
- raw: Commute
  sport_type: cycling
  modifiers: {is_commute: true}
`), "strava", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err == nil {
		t.Fatal("LoadSourceAliases accepted is_commute modifier")
	}
}

func TestLoadSourceAliasesKeepsNamespacesSeparate(t *testing.T) {
	readers := map[string]io.Reader{
		"gpx": strings.NewReader(`
source: gpx
aliases:
- raw: ride
  sport_type: cycling
`),
		"tcx": strings.NewReader(`
source: tcx
aliases:
- raw: ride
  sport_type: cycling
`),
	}
	sources, err := LoadSourceAliasFiles(readers, aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err != nil {
		t.Fatalf("LoadSourceAliasFiles() error = %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(sources))
	}
}

func TestRenderGoSourceAliasesIncludesSourcesAndPatches(t *testing.T) {
	source, err := LoadSourceAliases(strings.NewReader(`
source: strava
aliases:
- raw: Virtual Ride
  sport_type: cycling
  modifiers: {is_virtual: true}
- raw: E-Bike Ride
  sport_type: cycling
  modifiers: {is_electric: true}
- raw: Treadmill
  sport_type: running
  sub_type: treadmill
  modifiers: {is_indoor: true}
`), "strava", aliasTestTaxonomy(), modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err != nil {
		t.Fatalf("LoadSourceAliases() error = %v", err)
	}
	got, err := RenderGoSourceAliases([]SourceAliases{source}, modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err != nil {
		t.Fatalf("RenderGoSourceAliases() error = %v", err)
	}
	for _, want := range []string{
		"SourceStrava",
		`RawSport: "Virtual Ride"`,
		`SportType: "running", SubType: "treadmill"`,
		"IsVirtual: ptrBool(true)",
		"IsElectric: ptrBool(true)",
		"IsIndoor: ptrBool(true)",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("generated aliases missing %q:\n%s", want, got)
		}
	}
}

func TestRenderGoSourceAliasesSupportsRouteOnlyNamespace(t *testing.T) {
	got, err := RenderGoSourceAliases([]SourceAliases{{
		Source: "strava_route",
		Aliases: []SourceAlias{
			{Raw: "alpine ski", SportType: "alpine_skiing"},
		},
	}}, modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err != nil {
		t.Fatalf("RenderGoSourceAliases() error = %v", err)
	}
	if !bytes.Contains(got, []byte("SourceStravaRoute")) {
		t.Fatalf("generated aliases missing SourceStravaRoute:\n%s", got)
	}
}

func TestRenderTSSourceAliasesEmitsLegacyAliases(t *testing.T) {
	got, err := RenderTSSourceAliases([]SourceAliases{{
		Source: "legacy",
		Aliases: []SourceAlias{
			{Raw: "mountain_biking", SportType: "cycling", SubType: "mountain"},
			{Raw: "e_bike", SportType: "cycling", Modifiers: SourceAliasModifiers{"is_electric": ptrBoolForTest(true)}},
			{Raw: "wheelchair_push_walk", RawSub: "indoor_wheelchair_walk", SportType: "wheelchair_push", SubType: "walk_speed", Modifiers: SourceAliasModifiers{"is_indoor": ptrBoolForTest(true)}},
		},
	}}, modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err != nil {
		t.Fatalf("RenderTSSourceAliases() error = %v", err)
	}
	for _, want := range []string{
		"GENERATED_LEGACY_ALIASES",
		"{ raw: 'mountain_biking', sportType: 'cycling', subType: 'mountain' }",
		"{ raw: 'e_bike', sportType: 'cycling', modifiers: { isElectric: true } }",
		"GENERATED_LEGACY_SUBTYPE_ALIASES",
		"{ raw: 'wheelchair_push_walk', rawSub: 'indoor_wheelchair_walk', sportType: 'wheelchair_push', subType: 'walk_speed', modifiers: { isIndoor: true } }",
	} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("generated TS aliases missing %q:\n%s", want, got)
		}
	}
}

func TestRenderGoSourceAliasesRejectsUnknownNamespace(t *testing.T) {
	_, err := RenderGoSourceAliases([]SourceAliases{{
		Source:  "not_a_source",
		Aliases: []SourceAlias{{Raw: "Ride", SportType: "cycling"}},
	}}, modifierRegistryForTest(), sourceAliasPredicateForTest())
	if err == nil {
		t.Fatal("RenderGoSourceAliases accepted unknown namespace")
	}
}

func TestBuildGearDefaultProjectionCollapsesAliasesToTopLevelSports(t *testing.T) {
	projection, err := BuildGearDefaultProjection(aliasTestTaxonomy(), []SourceAliases{{
		Source: "strava",
		Aliases: []SourceAlias{
			{Raw: "Mountain Bike Ride", SportType: "cycling", SubType: "mountain"},
			{Raw: "E-Bike Ride", SportType: "cycling", Modifiers: SourceAliasModifiers{"is_electric": ptrBoolForTest(true)}},
			{Raw: "Virtual Ride", SportType: "cycling", Modifiers: SourceAliasModifiers{"is_virtual": ptrBoolForTest(true)}},
			{Raw: "Badminton", SportType: "racket", SubType: "badminton"},
			{Raw: "Elliptical", SportType: "fitness_equipment", SubType: "elliptical"},
		},
	}}, gearDefaultProjectionConfigForTest("strava"))
	if err != nil {
		t.Fatalf("BuildGearDefaultProjection() error = %v", err)
	}
	got := gearDefaultAliasesByRaw(t, projection, "strava")
	for raw, want := range map[string]string{
		"Mountain Bike Ride": "cycling",
		"E-Bike Ride":        "cycling",
		"Virtual Ride":       "cycling",
		"Badminton":          "racket",
		"Elliptical":         "fitness_equipment",
	} {
		if got[raw] != want {
			t.Fatalf("gear default raw %q = %q, want %q; all aliases = %#v", raw, got[raw], want, got)
		}
	}
}

func TestBuildGearDefaultProjectionProjectsOnlyStrava(t *testing.T) {
	projection, err := BuildGearDefaultProjection(aliasTestTaxonomy(), []SourceAliases{
		{Source: "strava", Aliases: []SourceAlias{{Raw: "Ride", SportType: "cycling"}}},
		{Source: "strava_route", Aliases: []SourceAlias{{Raw: "ride", SportType: "cycling"}}},
		{Source: "tcx", Aliases: []SourceAlias{{Raw: "Biking", SportType: "cycling"}}},
		{Source: "komoot", Aliases: []SourceAlias{{Raw: "bike_touring", SportType: "cycling"}}},
	}, gearDefaultProjectionConfigForTest("strava"))
	if err != nil {
		t.Fatalf("BuildGearDefaultProjection() error = %v", err)
	}
	if len(projection.Sources) != 1 || projection.Sources[0].Source != "strava" {
		t.Fatalf("projected sources = %#v, want only strava", projection.Sources)
	}
}

func TestBuildGearDefaultProjectionSkipsRawSubAliases(t *testing.T) {
	projection, err := BuildGearDefaultProjection(aliasTestTaxonomy(), []SourceAliases{{
		Source: "strava",
		Aliases: []SourceAlias{
			{Raw: "Workout", SportType: "running"},
			{Raw: "Workout", RawSub: "trail", SportType: "running", SubType: "trail"},
		},
	}}, gearDefaultProjectionConfigForTest("strava"))
	if err != nil {
		t.Fatalf("BuildGearDefaultProjection() error = %v", err)
	}
	got := gearDefaultAliasesByRaw(t, projection, "strava")
	if len(got) != 1 || got["Workout"] != "running" {
		t.Fatalf("gear default aliases = %#v, want only raw-only Workout", got)
	}
}

func TestBuildGearDefaultProjectionRejectsConflictingRawMapping(t *testing.T) {
	_, err := BuildGearDefaultProjection(aliasTestTaxonomy(), []SourceAliases{{
		Source: "strava",
		Aliases: []SourceAlias{
			{Raw: "Workout", SportType: "cycling"},
			{Raw: " Workout ", SportType: "running"},
		},
	}}, gearDefaultProjectionConfigForTest("strava"))
	if err == nil {
		t.Fatal("BuildGearDefaultProjection accepted conflicting raw mappings")
	}
	for _, want := range []string{"strava", "Workout", "cycling", "running"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestBuildGearDefaultProjectionOptionsSeparateStorageAndManualChoices(t *testing.T) {
	projection, err := BuildGearDefaultProjection(aliasTestTaxonomy(), nil, gearDefaultProjectionConfigForTest("strava"))
	if err != nil {
		t.Fatalf("BuildGearDefaultProjection() error = %v", err)
	}
	storage := gearDefaultOptionsBySportType(projection.StorageOptions)
	manual := gearDefaultOptionsBySportType(projection.ManualOptions)
	for _, want := range []string{"cycling", "generic", "fitness_equipment"} {
		if _, ok := storage[want]; !ok {
			t.Fatalf("storage options missing %q: %#v", want, storage)
		}
	}
	if _, ok := storage["unknown"]; ok {
		t.Fatalf("storage options include unknown: %#v", storage)
	}
	if _, ok := manual["cycling"]; !ok {
		t.Fatalf("manual options missing cycling: %#v", manual)
	}
	for _, notWant := range []string{"unknown", "fitness_equipment"} {
		if _, ok := manual[notWant]; ok {
			t.Fatalf("manual options include %q: %#v", notWant, manual)
		}
	}
}

func TestBuildGearDefaultProjectionExcludesRouteOnlyAliases(t *testing.T) {
	projection, err := BuildGearDefaultProjection(aliasTestTaxonomy(), []SourceAliases{
		{Source: "strava", Aliases: []SourceAlias{{Raw: "Ride", SportType: "cycling"}}},
		{Source: "strava_route", Aliases: []SourceAlias{{Raw: "alpine ski", SportType: "alpine_skiing"}}},
	}, gearDefaultProjectionConfigForTest("strava"))
	if err != nil {
		t.Fatalf("BuildGearDefaultProjection() error = %v", err)
	}
	got := gearDefaultAliasesByRaw(t, projection, "strava")
	if len(got) != 1 || got["Ride"] != "cycling" {
		t.Fatalf("gear default aliases = %#v, want only Strava Ride", got)
	}
	if len(projection.Sources) != 1 || projection.Sources[0].Source != "strava" {
		t.Fatalf("projected sources = %#v, want only strava", projection.Sources)
	}
}

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

func TestBuildGearDefaultProjectionIncludesConfiguredGarminAliases(t *testing.T) {
	projection, err := BuildGearDefaultProjection(aliasTestTaxonomy(), []SourceAliases{
		{Source: "strava", Aliases: []SourceAlias{{Raw: "Ride", SportType: "cycling"}}},
		{Source: "garmin", Aliases: []SourceAlias{{Raw: "running", SportType: "running"}}},
	}, gearDefaultProjectionConfigForTest("strava", "garmin"))
	if err != nil {
		t.Fatalf("BuildGearDefaultProjection() error = %v", err)
	}
	if got := gearDefaultAliasesByRaw(t, projection, "garmin"); got["running"] != "running" {
		t.Fatalf("garmin gear default aliases = %#v, want running", got)
	}
}

func TestLoadGearDefaultProjectionConfigRejectsUnknownSource(t *testing.T) {
	_, err := LoadGearDefaultProjectionConfig(strings.NewReader(`projection_sources: [garmin]`), []SourceAliases{{Source: "strava"}})
	if err == nil || !strings.Contains(err.Error(), "unknown projection source") {
		t.Fatalf("LoadGearDefaultProjectionConfig() error = %v, want unknown projection source", err)
	}
}

func TestLoadGearDefaultProjectionConfigRejectsDuplicateSource(t *testing.T) {
	_, err := LoadGearDefaultProjectionConfig(strings.NewReader(`projection_sources: [strava, strava]`), []SourceAliases{{Source: "strava"}})
	if err == nil || !strings.Contains(err.Error(), "duplicate projection source") {
		t.Fatalf("LoadGearDefaultProjectionConfig() error = %v, want duplicate projection source", err)
	}
}

func TestLoadGearDefaultProjectionConfigRejectsEmptySourceList(t *testing.T) {
	_, err := LoadGearDefaultProjectionConfig(strings.NewReader(`projection_sources: []`), []SourceAliases{{Source: "strava"}})
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("LoadGearDefaultProjectionConfig() error = %v, want empty list rejection", err)
	}
}

func TestCurrentAliasesRouteFitnessActivitiesToCanonicalSports(t *testing.T) {
	taxonomyFile, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer taxonomyFile.Close()
	taxonomy, err := LoadTaxonomy(taxonomyFile)
	if err != nil {
		t.Fatalf("load sports.yaml: %v", err)
	}

	for _, tc := range []struct {
		source    string
		raw       string
		wantSport string
	}{
		{source: "gpx", raw: "yoga", wantSport: "yoga"},
		{source: "gpx", raw: "pilates", wantSport: "pilates"},
		{source: "gpx", raw: "hiit", wantSport: "hiit"},
		{source: "strava", raw: "Yoga", wantSport: "yoga"},
		{source: "strava", raw: "Pilates", wantSport: "pilates"},
		{source: "strava", raw: "HIIT", wantSport: "hiit"},
	} {
		t.Run(tc.source+"/"+tc.raw, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "..", "config", "sport-taxonomy", "aliases", tc.source+".yaml")
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open aliases: %v", err)
			}
			defer file.Close()
			aliases, err := LoadSourceAliases(file, tc.source, taxonomy, modifierRegistryForTest(), sourceAliasPredicateForTest())
			if err != nil {
				t.Fatalf("LoadSourceAliases() error = %v", err)
			}
			for _, alias := range aliases.Aliases {
				if alias.Raw == tc.raw {
					if alias.SportType != tc.wantSport || alias.SubType != "" {
						t.Fatalf("alias %q = %s/%s, want %s with no subtype", tc.raw, alias.SportType, alias.SubType, tc.wantSport)
					}
					return
				}
			}
			t.Fatalf("alias %q not found in %s", tc.raw, tc.source)
		})
	}
}

func TestAliasesSchemaDocumentsSupportedModifierContract(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "..", "..", "config", "sport-taxonomy", "schema", "aliases.schema.json")
	src, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read aliases schema: %v", err)
	}
	schema := string(src)
	for _, want := range []string{
		`"additionalProperties": false`,
		`"is_electric"`,
		`"is_indoor"`,
		`"is_virtual"`,
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("aliases schema missing %s", want)
		}
	}
	if strings.Contains(schema, `"is_commute"`) {
		t.Fatal("aliases schema must not allow is_commute in this plan")
	}
}

func aliasTestTaxonomy() Taxonomy {
	selectable := true
	notSelectable := false
	return Taxonomy{
		Sports: []Sport{
			{SportType: "cycling", Labels: LocaleMap{"de": "Radfahren"}, Selectable: &selectable, SubTypes: []SubType{{SubType: "mountain"}, {SubType: "indoor_cycling"}}},
			{SportType: "running", Labels: LocaleMap{"de": "Laufen"}, Selectable: &selectable, SubTypes: []SubType{{SubType: "trail"}, {SubType: "treadmill"}}},
			{SportType: "swimming", Labels: LocaleMap{"de": "Schwimmen"}, Selectable: &selectable, SubTypes: []SubType{{SubType: "open_water"}}},
			{SportType: "racket", Labels: LocaleMap{"de": "Schlägersport"}, Selectable: &selectable, SubTypes: []SubType{{SubType: "badminton"}, {SubType: "squash"}}},
			{SportType: "fitness_equipment", Labels: LocaleMap{"de": "Fitnessgerät"}, Selectable: &notSelectable, SubTypes: []SubType{{SubType: "elliptical"}}},
			{SportType: "alpine_skiing", Labels: LocaleMap{"de": "Alpin-Ski"}, Selectable: &selectable},
			{SportType: "generic", Labels: LocaleMap{"de": "Aktivität"}, Selectable: &selectable},
			{SportType: "unknown", Labels: LocaleMap{"de": "Unbekannt"}, Selectable: &notSelectable},
		},
	}
}

func gearDefaultAliasesByRaw(t *testing.T, projection GearDefaultProjection, source string) map[string]string {
	t.Helper()
	for _, candidate := range projection.Sources {
		if candidate.Source != source {
			continue
		}
		out := make(map[string]string, len(candidate.Aliases))
		for _, alias := range candidate.Aliases {
			out[alias.Raw] = alias.SportType
		}
		return out
	}
	t.Fatalf("source %q not found in projection %#v", source, projection.Sources)
	return nil
}

func gearDefaultOptionsBySportType(options []GearDefaultSportOption) map[string]GearDefaultSportOption {
	out := make(map[string]GearDefaultSportOption, len(options))
	for _, option := range options {
		out[option.SportType] = option
	}
	return out
}

func gearDefaultProjectionConfigForTest(sources ...string) GearDefaultProjectionConfig {
	return GearDefaultProjectionConfig{ProjectionSources: sources}
}

func ptrBoolForTest(v bool) *bool {
	return &v
}
