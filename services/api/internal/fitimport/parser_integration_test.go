package fitimport

import (
	"math"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// TestBuildActivityData_EndToEnd creates a minimal proto.FIT with Session,
// Record, Lap, Event, Set, and Jump messages, then runs buildActivityData
// and verifies that ActivityData is populated correctly including all new
// FIT SDK extractor fields.
func TestBuildActivityData_EndToEnd(t *testing.T) {
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	end := now.Add(30 * time.Minute)

	// --- FileId ---
	fileId := &mesgdef.FileId{
		Type:         typedef.FileActivity,
		Manufacturer: typedef.ManufacturerGarmin,
	}

	// --- Session (use struct literal — NewSession(nil) sets all to invalid and
	//     ToMesg relies on internal state tracking which doesn't work after field mutation) ---
	session := &mesgdef.Session{
		StartTime:        now,
		Timestamp:        end,
		Sport:            typedef.SportCycling,
		SubSport:         typedef.SubSportGeneric,
		TotalElapsedTime: 1800000, // 1800s
		TotalTimerTime:   1700000, // 1700s
		TotalDistance:    2000000, // 20000m
		TotalAscent:      350,
		TotalDescent:     300,
		EnhancedAvgSpeed: 11111,
		EnhancedMaxSpeed: 16000,
		AvgHeartRate:     145,
		MaxHeartRate:     180,
		AvgPower:         220,
		MaxPower:         450,
		AvgCadence:       85,
		AvgTemperature:   22,
		TotalCalories:    800,
	}

	// --- Records (spread over 30 min to avoid CalcRecordingTime capping) ---
	rec1 := &mesgdef.Record{
		Timestamp:        now.Add(1 * time.Minute),
		PositionLat:      554680000, // ~46.5 deg
		PositionLong:     167400000, // ~14.0 deg
		EnhancedAltitude: 5000,      // /5 - 500 = 500m
		EnhancedSpeed:    5000,      // 5.0 m/s
		Distance:         500,       // 5.0 m
		HeartRate:        140,
		Power:            200,
		Cadence:          80,
		Temperature:      22,
		BatterySoc:       190, // /2 = 95.0
		Po2:              21,  // /100 = 0.21
		GpsAccuracy:      3,
	}

	rec2 := &mesgdef.Record{
		Timestamp:        now.Add(29 * time.Minute),
		PositionLat:      554690000,
		PositionLong:     167410000,
		EnhancedAltitude: 5010,
		EnhancedSpeed:    5500,
		Distance:         1000,
		HeartRate:        145,
		Power:            210,
		Cadence:          82,
		Temperature:      22,
	}

	// --- Lap ---
	lap := &mesgdef.Lap{
		StartTime:        now,
		Timestamp:        end,
		Sport:            typedef.SportCycling,
		SubSport:         typedef.SubSportGeneric,
		LapTrigger:       typedef.LapTriggerSessionEnd,
		TotalElapsedTime: 1800000,
		TotalTimerTime:   1700000,
		TotalDistance:    2000000,
		TotalAscent:      350,
		TotalDescent:     300,
		TotalCalories:    800,
		AvgHeartRate:     145,
		MaxHeartRate:     180,
		AvgCadence:       85,
		AvgPower:         220,
		MaxPower:         450,
		EnhancedAvgSpeed: 11111,
		AvgGrade:         500, // /100 = 5.0%
		AvgPosGrade:      800, // /100 = 8.0%
	}

	// --- Event ---
	event := &mesgdef.Event{
		Timestamp:                   now,
		Event:                       typedef.Event(0), // timer
		EventType:                   typedef.EventType(0),
		Data:                        99,
		FrontGearNum:                2,
		FrontGear:                   basetype.Uint8zInvalid,
		RearGearNum:                 basetype.Uint8zInvalid,
		RearGear:                    basetype.Uint8zInvalid,
		RadarThreatLevelMax:         typedef.RadarThreatLevelTypeInvalid,
		RadarThreatCount:            basetype.Uint8Invalid,
		RadarThreatAvgApproachSpeed: basetype.Uint8Invalid,
		RadarThreatMaxApproachSpeed: basetype.Uint8Invalid,
		Score:                       basetype.Uint16Invalid,
		OpponentScore:               basetype.Uint16Invalid,
		DeviceIndex:                 typedef.DeviceIndexInvalid,
		ActivityType:                typedef.ActivityTypeInvalid,
	}

	// --- Set ---
	set := &mesgdef.Set{
		Timestamp:   now.Add(5 * time.Minute),
		Weight:      1600, // /16 = 100.0 kg
		Repetitions: 10,
		SetType:     typedef.SetType(0),
	}

	// --- Jump ---
	jump := &mesgdef.Jump{
		Timestamp:     now.Add(10 * time.Minute),
		Height:        2.5,
		EnhancedSpeed: 5000, // /1000 = 5.0
	}

	// --- Build proto.FIT ---
	opts := mesgdef.DefaultOptions()
	fit := &proto.FIT{
		Messages: []proto.Message{
			fileId.ToMesg(opts),
			session.ToMesg(opts),
			rec1.ToMesg(opts),
			rec2.ToMesg(opts),
			lap.ToMesg(opts),
			event.ToMesg(opts),
			set.ToMesg(opts),
			jump.ToMesg(opts),
		},
	}

	// --- Run buildActivityData ---
	data, err := buildActivityData(fit, "test_e2e.fit")
	if err != nil {
		t.Fatalf("buildActivityData failed: %v", err)
	}

	// --- Verify basic fields ---
	if data.SportType != "cycling" {
		t.Errorf("SportType: got %q, want 'cycling'", data.SportType)
	}
	if data.FileFormat != "fit" {
		t.Errorf("FileFormat: got %q, want 'fit'", data.FileFormat)
	}
	if data.OriginalName != "test_e2e.fit" {
		t.Errorf("OriginalName: got %q, want 'test_e2e.fit'", data.OriginalName)
	}
	if !data.StartedAt.Equal(now) {
		t.Errorf("StartedAt: got %v, want %v", data.StartedAt, now)
	}
	if data.ElapsedTimeS != 1800 {
		t.Errorf("ElapsedTimeS: got %d, want 1800", data.ElapsedTimeS)
	}
	if math.Abs(data.DistanceM-20000.0) > 0.1 {
		t.Errorf("DistanceM: got %f, want 20000.0", data.DistanceM)
	}
	if data.TotalCalories != 800 {
		t.Errorf("TotalCalories: got %d, want 800", data.TotalCalories)
	}

	// --- Verify streams ---
	if len(data.Streams) < 2 {
		t.Fatalf("Streams: got %d, want >= 2", len(data.Streams))
	}
	sp := data.Streams[0]
	if sp.BatterySocPct != nil && math.Abs(*sp.BatterySocPct-95.0) > 0.1 {
		t.Errorf("Stream[0].BatterySocPct: got %f, want 95.0", *sp.BatterySocPct)
	}
	if sp.Po2 != nil && math.Abs(*sp.Po2-0.21) > 0.01 {
		t.Errorf("Stream[0].Po2: got %f, want 0.21", *sp.Po2)
	}
	if sp.GpsAccuracyM == nil || *sp.GpsAccuracyM != 3 {
		t.Errorf("Stream[0].GpsAccuracyM: got %v, want 3", sp.GpsAccuracyM)
	}

	// --- Verify laps ---
	if len(data.Laps) != 1 {
		t.Fatalf("Laps: got %d, want 1", len(data.Laps))
	}
	lapRec := data.Laps[0]
	if lapRec.AvgGradePct == nil || *lapRec.AvgGradePct != 5.0 {
		t.Errorf("Lap.AvgGradePct: got %v, want 5.0", lapRec.AvgGradePct)
	}
	if lapRec.AvgPosGradePct == nil || *lapRec.AvgPosGradePct != 8.0 {
		t.Errorf("Lap.AvgPosGradePct: got %v, want 8.0", lapRec.AvgPosGradePct)
	}

	// --- Verify events ---
	if len(data.Events) != 1 {
		t.Fatalf("Events: got %d, want 1", len(data.Events))
	}
	ev := data.Events[0]
	if ev.Event != 0 {
		t.Errorf("Event.Event: got %d, want 0", ev.Event)
	}
	if ev.Data == nil || *ev.Data != 99 {
		t.Errorf("Event.Data: got %v, want 99", ev.Data)
	}
	if ev.FrontGearNum == nil || *ev.FrontGearNum != 2 {
		t.Errorf("Event.FrontGearNum: got %v, want 2", ev.FrontGearNum)
	}

	// --- Verify sets ---
	if len(data.Sets) != 1 {
		t.Fatalf("Sets: got %d, want 1", len(data.Sets))
	}
	setRec := data.Sets[0]
	if setRec.WeightKg == nil || *setRec.WeightKg != 100.0 {
		t.Errorf("Set.WeightKg: got %v, want 100.0", setRec.WeightKg)
	}
	if setRec.Repetitions == nil || *setRec.Repetitions != 10 {
		t.Errorf("Set.Repetitions: got %v, want 10", setRec.Repetitions)
	}

	// --- Verify jumps ---
	if len(data.Jumps) != 1 {
		t.Fatalf("Jumps: got %d, want 1", len(data.Jumps))
	}
	jumpRec := data.Jumps[0]
	if jumpRec.HeightM == nil || *jumpRec.HeightM != 2.5 {
		t.Errorf("Jump.HeightM: got %v, want 2.5", jumpRec.HeightM)
	}
	if jumpRec.SpeedMs == nil || *jumpRec.SpeedMs != 5.0 {
		t.Errorf("Jump.SpeedMs: got %v, want 5.0", jumpRec.SpeedMs)
	}
}
