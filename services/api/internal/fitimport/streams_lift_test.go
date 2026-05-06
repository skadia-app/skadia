package fitimport

import (
	"math"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/mesgdef"
)

func TestExtractStreamsLiftSpeedUsesEnhancedSpeedBeforeSpeed(t *testing.T) {
	streams := ExtractStreams([]*mesgdef.Record{{
		Timestamp:        time.Unix(0, 0).UTC(),
		PositionLat:      invalidSint32,
		PositionLong:     invalidSint32,
		EnhancedSpeed:    3500,
		Speed:            9000,
		EnhancedAltitude: 0xFFFFFFFF,
		Altitude:         0xFFFF,
	}}, nil)
	if len(streams) != 1 {
		t.Fatalf("stream count = %d, want 1", len(streams))
	}
	if streams[0].SpeedMS == nil || math.Abs(*streams[0].SpeedMS-3.5) > 0.001 {
		t.Fatalf("SpeedMS = %v, want 3.5 from EnhancedSpeed", streams[0].SpeedMS)
	}
}

func TestExtractStreamsLiftSpeedFallsBackToSpeed(t *testing.T) {
	streams := ExtractStreams([]*mesgdef.Record{{
		Timestamp:        time.Unix(0, 0).UTC(),
		PositionLat:      invalidSint32,
		PositionLong:     invalidSint32,
		EnhancedSpeed:    0xFFFFFFFF,
		Speed:            2100,
		EnhancedAltitude: 0xFFFFFFFF,
		Altitude:         0xFFFF,
	}}, nil)
	if len(streams) != 1 {
		t.Fatalf("stream count = %d, want 1", len(streams))
	}
	if streams[0].SpeedMS == nil || math.Abs(*streams[0].SpeedMS-2.1) > 0.001 {
		t.Fatalf("SpeedMS = %v, want 2.1 from Speed fallback", streams[0].SpeedMS)
	}
}

func TestExtractStreamsLiftSpeedSkipsInvalidSentinels(t *testing.T) {
	streams := ExtractStreams([]*mesgdef.Record{{
		Timestamp:        time.Unix(0, 0).UTC(),
		PositionLat:      invalidSint32,
		PositionLong:     invalidSint32,
		EnhancedSpeed:    0xFFFFFFFF,
		Speed:            0xFFFF,
		EnhancedAltitude: 0xFFFFFFFF,
		Altitude:         0xFFFF,
	}}, nil)
	if len(streams) != 1 {
		t.Fatalf("stream count = %d, want 1", len(streams))
	}
	if streams[0].SpeedMS != nil {
		t.Fatalf("SpeedMS = %v, want nil for invalid speed sentinels", *streams[0].SpeedMS)
	}
}
