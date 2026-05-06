package sidecar

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestGenerateMetaFullUsesFinalActivityTupleAndCommuteFlag(t *testing.T) {
	subType := "mountain"
	title := "Final Activity Title"
	data := &model.ActivityData{
		OriginalName: "Imported Source Title",
		SportType:    "cycling",
		SubType:      "road",
		StartedAt:    time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
	}
	activity := &model.Activity{
		ID:            "act-1",
		UserID:        "user-1",
		Title:         &title,
		SportType:     "cycling",
		SubType:       &subType,
		IsCommute:     true,
		IsIndoor:      true,
		IsElectric:    true,
		Visibility:    "private",
		CreatedAt:     time.Date(2026, 5, 1, 10, 1, 0, 0, time.UTC),
		TitleIsManual: true,
	}

	meta := GenerateMetaFull("act-1", data, activity)

	if meta.SportType != "cycling" || meta.SubType != "mountain" {
		t.Fatalf("meta tuple = %s/%s, want final activity cycling/mountain", meta.SportType, meta.SubType)
	}
	if meta.Title != "Final Activity Title" {
		t.Fatalf("meta.Title = %q, want final manual activity title", meta.Title)
	}
	if !meta.IsCommute {
		t.Fatal("meta.IsCommute = false, want true")
	}
	if !meta.IsIndoor || !meta.IsElectric {
		t.Fatalf("meta modifiers indoor/electric = %v/%v, want true/true", meta.IsIndoor, meta.IsElectric)
	}
}
