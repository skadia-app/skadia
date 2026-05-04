package shadowfs

import (
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
		"duration_h":  "1-30",
		"elevation_m": "321",
	}
	for key, wantValue := range want {
		if tags[key] != wantValue {
			t.Fatalf("%s = %q, want %q in tags %#v", key, tags[key], wantValue, tags)
		}
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
