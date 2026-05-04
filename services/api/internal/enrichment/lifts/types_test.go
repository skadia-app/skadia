package lifts_test

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

func TestStreamPointAndRide(t *testing.T) {
	now := time.Now()

	// StreamPoint must have all required nullable fields.
	lat := 47.0
	lng := 12.5
	alt := 1800.0
	spd := 5.5
	dist := 1200.0
	pt := lifts.StreamPoint{
		Time:      now,
		Lat:       &lat,
		Lng:       &lng,
		AltitudeM: &alt,
		SpeedMs:   &spd,
		DistanceM: &dist,
	}
	if pt.Time.IsZero() {
		t.Error("expected non-zero time")
	}
	if pt.AltitudeM == nil {
		t.Error("expected non-nil AltitudeM")
	}

	// LiftRide.Validate() must return nil when end >= start.
	validRide := lifts.LiftRide{
		StartedAt:  now,
		EndedAt:    now.Add(5 * time.Minute),
		DurationS:  300,
		AscentM:    400,
		DescentM:   0,
		DistanceM:  1200,
		Confidence: "high",
	}
	if err := validRide.Validate(); err != nil {
		t.Errorf("valid ride returned error: %v", err)
	}

	// LiftRide.Validate() must return an error when EndedAt < StartedAt.
	badRide := lifts.LiftRide{
		StartedAt: now.Add(10 * time.Minute),
		EndedAt:   now,
	}
	if err := badRide.Validate(); err == nil {
		t.Error("expected error for ride with end before start, got nil")
	}
}
