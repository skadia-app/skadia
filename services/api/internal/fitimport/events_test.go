package fitimport

import (
	"testing"
	"time"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

func TestExtractEvents_BasicFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	events := []*mesgdef.Event{
		{
			Timestamp:                   now,
			Event:                       typedef.Event(42),
			EventType:                   typedef.EventType(0),
			Data:                        1234567890,
			EventGroup:                  5,
			FrontGearNum:                2,
			FrontGear:                   basetype.Uint8zInvalid, // skip
			RearGearNum:                 basetype.Uint8zInvalid, // skip
			RearGear:                    basetype.Uint8zInvalid, // skip
			RadarThreatLevelMax:         typedef.RadarThreatLevelTypeInvalid,
			RadarThreatCount:            basetype.Uint8Invalid,
			RadarThreatAvgApproachSpeed: 50,
			RadarThreatMaxApproachSpeed: basetype.Uint8Invalid,
			Score:                       basetype.Uint16Invalid,
			OpponentScore:               basetype.Uint16Invalid,
			DeviceIndex:                 typedef.DeviceIndexInvalid,
			ActivityType:                typedef.ActivityTypeInvalid,
		},
	}

	result := ExtractEvents(events)
	if len(result) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result))
	}

	rec := result[0]
	if rec.Event != 42 {
		t.Errorf("Event: got %d, want 42", rec.Event)
	}
	if rec.EventType != 0 {
		t.Errorf("EventType: got %d, want 0", rec.EventType)
	}
	if rec.Data == nil || *rec.Data != 1234567890 {
		t.Errorf("Data: got %v, want 1234567890", rec.Data)
	}
	if rec.EventGroup == nil || *rec.EventGroup != 5 {
		t.Errorf("EventGroup: got %v, want 5", rec.EventGroup)
	}
	if rec.FrontGearNum == nil || *rec.FrontGearNum != 2 {
		t.Errorf("FrontGearNum: got %v, want 2", rec.FrontGearNum)
	}
	if rec.FrontGear != nil {
		t.Errorf("FrontGear: expected nil, got %v", *rec.FrontGear)
	}

	// RadarThreatAvgApproachSpeed: 50 / 10.0 = 5.0
	if rec.RadarThreatAvgApproachSpeedMs == nil || *rec.RadarThreatAvgApproachSpeedMs != 5.0 {
		t.Errorf("RadarThreatAvgApproachSpeedMs: got %v, want 5.0", rec.RadarThreatAvgApproachSpeedMs)
	}
	if rec.RadarThreatMaxApproachSpeedMs != nil {
		t.Errorf("RadarThreatMaxApproachSpeedMs: expected nil")
	}
}

func TestExtractEvents_SkipsInvalidEvent(t *testing.T) {
	events := []*mesgdef.Event{
		{Event: typedef.EventInvalid},
	}
	result := ExtractEvents(events)
	if len(result) != 0 {
		t.Errorf("expected 0 events for invalid Event, got %d", len(result))
	}
}

func TestExtractEvents_ScoresAndStartTimestamp(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-10 * time.Minute)

	events := []*mesgdef.Event{
		{
			Timestamp:                   now,
			Event:                       typedef.Event(1),
			EventType:                   typedef.EventType(0),
			Data:                        basetype.Uint32Invalid,
			EventGroup:                  basetype.Uint8Invalid,
			FrontGearNum:                basetype.Uint8zInvalid,
			FrontGear:                   basetype.Uint8zInvalid,
			RearGearNum:                 basetype.Uint8zInvalid,
			RearGear:                    basetype.Uint8zInvalid,
			RadarThreatLevelMax:         typedef.RadarThreatLevelTypeInvalid,
			RadarThreatCount:            basetype.Uint8Invalid,
			RadarThreatAvgApproachSpeed: basetype.Uint8Invalid,
			RadarThreatMaxApproachSpeed: basetype.Uint8Invalid,
			Score:                       100,
			OpponentScore:               80,
			DeviceIndex:                 typedef.DeviceIndex(0),
			ActivityType:                typedef.ActivityType(1),
			StartTimestamp:              start,
		},
	}

	result := ExtractEvents(events)
	if len(result) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result))
	}
	rec := result[0]

	if rec.Score == nil || *rec.Score != 100 {
		t.Errorf("Score: got %v, want 100", rec.Score)
	}
	if rec.OpponentScore == nil || *rec.OpponentScore != 80 {
		t.Errorf("OpponentScore: got %v, want 80", rec.OpponentScore)
	}
	if rec.DeviceIndex == nil || *rec.DeviceIndex != 0 {
		t.Errorf("DeviceIndex: got %v, want 0", rec.DeviceIndex)
	}
	if rec.ActivityType == nil || *rec.ActivityType != 1 {
		t.Errorf("ActivityType: got %v, want 1", rec.ActivityType)
	}
	if rec.StartTimestamp == nil || !rec.StartTimestamp.Equal(start) {
		t.Errorf("StartTimestamp: got %v, want %v", rec.StartTimestamp, start)
	}
}
