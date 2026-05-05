package importer

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestMultisportActivitySchemaTagsInputAggregatesSessionFields(t *testing.T) {
	start := time.Date(2026, 5, 3, 7, 45, 0, 0, time.UTC)
	sessions := []*model.ActivityData{
		{
			SportType:      "swimming",
			StartedAt:      start.Add(20 * time.Minute),
			DistanceM:      1500,
			ElapsedTimeS:   1800,
			ElevationGainM: 4,
			IsIndoor:       true,
		},
		{
			SportType:      "cycling",
			StartedAt:      start,
			DistanceM:      40000,
			ElapsedTimeS:   5400,
			ElevationGainM: 350,
			IsElectric:     true,
		},
		{
			SportType:      "running",
			StartedAt:      start.Add(2 * time.Hour),
			DistanceM:      10000,
			ElapsedTimeS:   3000,
			ElevationGainM: 80,
			IsVirtual:      true,
		},
	}

	got := MultisportActivitySchemaTagsInput("fit", "upload", sessions)

	if got.Format != "fit" || got.Source != "upload" || got.SportType != "multisport" {
		t.Fatalf("identity tags = format %q source %q sport %q, want fit/upload/multisport", got.Format, got.Source, got.SportType)
	}
	if !got.StartedAt.Equal(start) {
		t.Fatalf("StartedAt = %s, want %s", got.StartedAt, start)
	}
	if got.DistanceM != 51500 {
		t.Fatalf("DistanceM = %v, want 51500", got.DistanceM)
	}
	if got.ElapsedTimeS != 10200 {
		t.Fatalf("ElapsedTimeS = %d, want 10200", got.ElapsedTimeS)
	}
	if got.ElevationGainM != 434 {
		t.Fatalf("ElevationGainM = %v, want 434", got.ElevationGainM)
	}
	if !got.IsElectric || !got.IsIndoor || !got.IsVirtual {
		t.Fatalf("modifiers = electric:%v indoor:%v virtual:%v, want all true", got.IsElectric, got.IsIndoor, got.IsVirtual)
	}
}
