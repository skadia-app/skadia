package shadowfs

import (
	"math"
	"testing"
	"time"
)

func TestActivitySchemaTagsNormalizesLegacySportTuple(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{
		Format:    "fit",
		Source:    "upload",
		SportType: "mountain_biking",
	})

	if tags["format"] != "fit" || tags["source"] != "upload" {
		t.Fatalf("format/source tags = %#v", tags)
	}
	if tags["sport"] != "cycling" || tags["subCategory"] != "mountain" {
		t.Fatalf("sport tags = %#v, want sport=cycling subCategory=mountain", tags)
	}
}

func TestActivitySchemaTagsOmitsMissingSubCategory(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{SportType: "cycling"})

	if tags["sport"] != "cycling" {
		t.Fatalf("sport = %q, want cycling", tags["sport"])
	}
	if _, ok := tags["subCategory"]; ok {
		t.Fatalf("subCategory should be omitted for top-level sport: %#v", tags)
	}
}

func TestActivitySchemaTagsMultisportParent(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{SportType: "multisport", SubType: "ignored"})

	if tags["sport"] != "multisport" {
		t.Fatalf("sport = %q, want multisport", tags["sport"])
	}
	if _, ok := tags["subCategory"]; ok {
		t.Fatalf("subCategory should be omitted for multisport parent: %#v", tags)
	}
}

func TestActivitySchemaTagsMetadata(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{
		StartedAt:      time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
		SportType:      "running",
		Title:          "Lunch Run",
		City:           "Berlin",
		Country:        "DE",
		DistanceM:      12345,
		ElapsedTimeS:   5432,
		ElevationGainM: 321.4,
	})

	want := map[string]string{
		"year":        "2024",
		"month":       "03",
		"day":         "15",
		"time":        "1430",
		"sport":       "running",
		"title":       "Lunch Run",
		"city":        "Berlin",
		"country":     "DE",
		"distance_km": "12.3",
		"duration_h":  "0001-30",
		"elevation_m": "321",
	}
	for key, wantValue := range want {
		if tags[key] != wantValue {
			t.Fatalf("%s = %q, want %q in tags %#v", key, tags[key], wantValue, tags)
		}
	}
}

func TestActivitySchemaTagsDurationSortsLexicographically(t *testing.T) {
	short := ActivitySchemaTags(ActivitySchemaTagsInput{SportType: "running", ElapsedTimeS: 2 * 3600})
	long := ActivitySchemaTags(ActivitySchemaTagsInput{SportType: "running", ElapsedTimeS: 10 * 3600})

	if short["duration_h"] != "0002-00" {
		t.Fatalf("short duration_h = %q, want 0002-00", short["duration_h"])
	}
	if long["duration_h"] != "0010-00" {
		t.Fatalf("long duration_h = %q, want 0010-00", long["duration_h"])
	}
	if !(short["duration_h"] < long["duration_h"]) {
		t.Fatalf("duration_h values do not sort lexicographically: %q >= %q", short["duration_h"], long["duration_h"])
	}
}

func TestActivitySchemaTagsSkipsNonFiniteMetrics(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{
		SportType:      "running",
		DistanceM:      math.Inf(1),
		ElevationGainM: math.NaN(),
		ElapsedTimeS:   3600,
	})

	if _, ok := tags["distance_km"]; ok {
		t.Fatalf("non-finite distance should be omitted: %#v", tags)
	}
	if _, ok := tags["elevation_m"]; ok {
		t.Fatalf("non-finite elevation should be omitted: %#v", tags)
	}
	if tags["duration_h"] != "0001-00" {
		t.Fatalf("duration_h = %q, want finite duration to remain", tags["duration_h"])
	}
}

func TestActivitySchemaTagsModifiersStableOrder(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{
		SportType:  "cycling",
		IsElectric: true,
		IsIndoor:   true,
		IsVirtual:  true,
		IsCommute:  true,
	})

	if tags["modifiers"] != "electric_indoor_virtual_commute" {
		t.Fatalf("modifiers = %q, want electric_indoor_virtual_commute", tags["modifiers"])
	}

	tags = ActivitySchemaTags(ActivitySchemaTagsInput{SportType: "running"})
	if got, ok := tags["modifiers"]; !ok || got != "" {
		t.Fatalf("inactive modifiers tag = %q present=%v, want present empty string", got, ok)
	}
}

func TestActivitySchemaTagsTrimsOptionalTextTags(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{
		SportType: "running",
		Title:     "   ",
		City:      "  Augsburg  ",
		Country:   "  DE  ",
	})

	if _, ok := tags["title"]; ok {
		t.Fatalf("whitespace-only title should be omitted: %#v", tags)
	}
	if tags["city"] != "Augsburg" || tags["country"] != "DE" {
		t.Fatalf("city/country tags should be trimmed: %#v", tags)
	}
}

func TestActivitySchemaTagsUnknownSportDoesNotLeakRawValue(t *testing.T) {
	tags := ActivitySchemaTags(ActivitySchemaTagsInput{SportType: "not_a_sport", SubType: "raw_sub"})

	if tags["sport"] != "unknown" {
		t.Fatalf("sport = %q, want unknown", tags["sport"])
	}
	if _, ok := tags["subCategory"]; ok {
		t.Fatalf("subCategory should be omitted for unknown raw sport: %#v", tags)
	}
	for _, value := range tags {
		if value == "not_a_sport" || value == "raw_sub" {
			t.Fatalf("raw taxonomy value leaked into tags: %#v", tags)
		}
	}
}
