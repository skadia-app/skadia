package handler

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestFusionActivitySchemaTagsInputUsesFinalMergedTuple(t *testing.T) {
	subType := "mountain"
	title := "Merged MTB"
	elapsed := 3600
	distance := 25000.0
	elevation := 800.0
	startCity := "Augsburg"
	startCountry := "DE"
	startedAt := time.Date(2026, 5, 3, 14, 30, 0, 0, time.UTC)

	input := fusionActivitySchemaTagsInput("fit", "fusion_merge", &model.Activity{
		StartedAt:      startedAt,
		SportType:      "cycling",
		SubType:        &subType,
		Title:          &title,
		StartCity:      &startCity,
		StartCountry:   &startCountry,
		ElapsedTimeS:   &elapsed,
		DistanceM:      &distance,
		ElevationGainM: &elevation,
	})

	if input.SportType != "cycling" {
		t.Fatalf("SportType = %q, want cycling", input.SportType)
	}
	if input.SubType != "mountain" {
		t.Fatalf("SubType = %q, want mountain", input.SubType)
	}
	if input.StartedAt != startedAt {
		t.Fatalf("StartedAt = %v, want %v", input.StartedAt, startedAt)
	}
	if input.ElapsedTimeS != elapsed {
		t.Fatalf("ElapsedTimeS = %d, want %d", input.ElapsedTimeS, elapsed)
	}
	if input.DistanceM != distance {
		t.Fatalf("DistanceM = %f, want %f", input.DistanceM, distance)
	}
	if input.ElevationGainM != elevation {
		t.Fatalf("ElevationGainM = %f, want %f", input.ElevationGainM, elevation)
	}
	if input.City != startCity || input.Country != startCountry {
		t.Fatalf("location tags = %q/%q, want %q/%q", input.City, input.Country, startCity, startCountry)
	}
}
