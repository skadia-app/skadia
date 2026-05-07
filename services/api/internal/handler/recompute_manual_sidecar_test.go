package handler

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/sidecar"
)

func TestActivityDataFromManualSidecarPreservesModifierState(t *testing.T) {
	startedAt := time.Date(2026, 5, 6, 12, 30, 0, 0, time.UTC)
	data := activityDataFromManualSidecar(&sidecar.ManualActivityFile{
		FileFormat:     "manual",
		Source:         "manual",
		SportType:      "cycling",
		SubType:        "mountain",
		StartedAt:      startedAt,
		ElapsedTimeS:   3600,
		MovingTimeS:    3500,
		DistanceM:      12345,
		ElevationGainM: 450,
		ElevationLossM: 440,
		IsElectric:     true,
		IsIndoor:       true,
		IsVirtual:      true,
		IsCommute:      true,
	})

	if !data.IsElectric || !data.IsIndoor || !data.IsVirtual || !data.IsCommute {
		t.Fatalf("modifiers = electric:%v indoor:%v virtual:%v commute:%v, want all true",
			data.IsElectric, data.IsIndoor, data.IsVirtual, data.IsCommute)
	}
	if !data.EndedAt.Equal(startedAt.Add(time.Hour)) {
		t.Fatalf("EndedAt = %v, want %v", data.EndedAt, startedAt.Add(time.Hour))
	}
}
