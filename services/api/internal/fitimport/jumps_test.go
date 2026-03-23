package fitimport

import (
	"math"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
)

func TestExtractJumps_EnhancedSpeedPriority(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	jumps := []*mesgdef.Jump{
		{
			Timestamp:     now,
			Distance:      10.5,
			Height:        2.5,
			HangTime:      1.2,
			Score:         95.5,
			EnhancedSpeed: 5000, // /1000 = 5.0 m/s
			Speed:         3000, // should be ignored because EnhancedSpeed is valid
			Rotations:     2,
			PositionLat:   basetype.Sint32Invalid,
			PositionLong:  basetype.Sint32Invalid,
		},
	}

	result := ExtractJumps(jumps)
	if len(result) != 1 {
		t.Fatalf("expected 1 jump, got %d", len(result))
	}

	rec := result[0]

	// EnhancedSpeed: 5000 / 1000.0 = 5.0
	if rec.SpeedMs == nil || *rec.SpeedMs != 5.0 {
		t.Errorf("SpeedMs: got %v, want 5.0", rec.SpeedMs)
	}

	// Height: 2.5 (float32 direct)
	if rec.HeightM == nil || *rec.HeightM != 2.5 {
		t.Errorf("HeightM: got %v, want 2.5", rec.HeightM)
	}

	// Distance
	if rec.DistanceM == nil || math.Abs(*rec.DistanceM-10.5) > 0.01 {
		t.Errorf("DistanceM: got %v, want 10.5", rec.DistanceM)
	}

	// HangTime
	if rec.HangTimeS == nil || math.Abs(*rec.HangTimeS-1.2) > 0.01 {
		t.Errorf("HangTimeS: got %v, want 1.2", rec.HangTimeS)
	}

	// Score
	if rec.Score == nil || math.Abs(*rec.Score-95.5) > 0.01 {
		t.Errorf("Score: got %v, want 95.5", rec.Score)
	}

	// Rotations
	if rec.Rotations == nil || *rec.Rotations != 2 {
		t.Errorf("Rotations: got %v, want 2", rec.Rotations)
	}

	// GPS should be nil (invalid)
	if rec.Lat != nil {
		t.Errorf("Lat: expected nil")
	}
	if rec.Lng != nil {
		t.Errorf("Lng: expected nil")
	}
}

func TestExtractJumps_SpeedFallback(t *testing.T) {
	jumps := []*mesgdef.Jump{
		{
			Timestamp:     time.Now().UTC(),
			Distance:      math.Float32frombits(basetype.Float32Invalid),
			Height:        math.Float32frombits(basetype.Float32Invalid),
			HangTime:      math.Float32frombits(basetype.Float32Invalid),
			Score:         math.Float32frombits(basetype.Float32Invalid),
			EnhancedSpeed: basetype.Uint32Invalid, // invalid → fallback to Speed
			Speed:         3000,                   // /1000 = 3.0
			Rotations:     basetype.Uint8Invalid,
			PositionLat:   basetype.Sint32Invalid,
			PositionLong:  basetype.Sint32Invalid,
		},
	}

	result := ExtractJumps(jumps)
	if len(result) != 1 {
		t.Fatalf("expected 1 jump, got %d", len(result))
	}

	rec := result[0]
	if rec.SpeedMs == nil || *rec.SpeedMs != 3.0 {
		t.Errorf("SpeedMs fallback: got %v, want 3.0", rec.SpeedMs)
	}

	// All float32 fields should be nil (invalid)
	if rec.DistanceM != nil {
		t.Errorf("DistanceM: expected nil for invalid float32")
	}
	if rec.HeightM != nil {
		t.Errorf("HeightM: expected nil for invalid float32")
	}
	if rec.HangTimeS != nil {
		t.Errorf("HangTimeS: expected nil for invalid float32")
	}
	if rec.Score != nil {
		t.Errorf("Score: expected nil for invalid float32")
	}
	if rec.Rotations != nil {
		t.Errorf("Rotations: expected nil for invalid")
	}
}
