package sporttaxgen

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

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
`), "strava", aliasTestTaxonomy())
	if err != nil {
		t.Fatalf("LoadSourceAliases() error = %v", err)
	}
	if got := source.Aliases[0].Modifiers.IsVirtual; got == nil || !*got {
		t.Fatalf("is_virtual patch = %#v, want true", got)
	}
}

func TestLoadSourceAliasesRejectsSourceFilenameMismatch(t *testing.T) {
	_, err := LoadSourceAliases(strings.NewReader(`
source: strava
aliases:
- raw: Ride
  sport_type: cycling
`), "gpx", aliasTestTaxonomy())
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
`), "gpx", aliasTestTaxonomy())
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
`), "gpx", aliasTestTaxonomy())
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
`), "gpx", aliasTestTaxonomy())
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
`), "gpx", aliasTestTaxonomy())
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
`), "strava", aliasTestTaxonomy())
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
	sources, err := LoadSourceAliasFiles(readers, aliasTestTaxonomy())
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
`), "strava", aliasTestTaxonomy())
	if err != nil {
		t.Fatalf("LoadSourceAliases() error = %v", err)
	}
	got, err := RenderGoSourceAliases([]SourceAliases{source})
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

func aliasTestTaxonomy() Taxonomy {
	return Taxonomy{
		Sports: []Sport{
			{SportType: "cycling", SubTypes: []SubType{{SubType: "mountain"}, {SubType: "indoor_cycling"}}},
			{SportType: "running", SubTypes: []SubType{{SubType: "trail"}, {SubType: "treadmill"}}},
			{SportType: "swimming", SubTypes: []SubType{{SubType: "open_water"}}},
		},
	}
}
