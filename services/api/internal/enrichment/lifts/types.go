package lifts

import (
	"errors"
	"time"
)

// StreamPoint is one sample of an activity stream as consumed by the lift
// detector. Pointer fields preserve the SQL NULL semantics of activity_streams.
// The detector treats nil as "no data" — never as zero.
type StreamPoint struct {
	Time      time.Time
	Lat       *float64
	Lng       *float64
	AltitudeM *float64
	SpeedMs   *float64
	DistanceM *float64
}

// LiftRide is one detected lift segment in the stream. Distance/ascent/descent
// are direction-agnostic totals (a downhill cable car contributes DescentM > 0
// and AscentM == 0; an upward gondola the inverse; a bidirectional ride like
// the bidirectional aerial tramway yields both > 0).
type LiftRide struct {
	StartedAt time.Time
	EndedAt   time.Time
	DurationS int
	AscentM   float64
	DescentM  float64
	DistanceM float64
	// Confidence levels: "high" (speed-stable + gradient-strict),
	// "medium" (no usable speed but altitude-rate strong), "low" (resume
	// gap inference; reserved for the pause-detector path).
	Confidence string
}

func (r LiftRide) Validate() error {
	if r.EndedAt.Before(r.StartedAt) {
		return errors.New("lifts: ride end before start")
	}
	return nil
}
