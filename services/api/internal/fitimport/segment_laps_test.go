package fitimport

import (
	"math"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

func TestExtractSegmentLaps_GradeAndPedalAnalytics(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	laps := []*mesgdef.SegmentLap{
		{
			Timestamp:    now,
			StartTime:    now.Add(-5 * time.Minute),
			MessageIndex: typedef.MessageIndex(0),
			Name:         "Test Segment",
			Uuid:         "abc-123",

			// Grade: 500 / 100 = 5.0%
			AvgGrade:    500,
			AvgPosGrade: 800,
			AvgNegGrade: basetype.Sint16Invalid, // skip
			MaxPosGrade: basetype.Sint16Invalid, // skip
			MaxNegGrade: basetype.Sint16Invalid, // skip

			// Torque Effectiveness: 180 / 2.0 = 90.0%
			AvgLeftTorqueEffectiveness:  180,
			AvgRightTorqueEffectiveness: basetype.Uint8Invalid, // skip

			// Power Phase: [0, 180] / 0.7111111 = [0.0, ~253.1]
			AvgLeftPowerPhase:     []uint8{0, 180},
			AvgLeftPowerPhasePeak: nil, // skip

			// Other fields — set to invalid
			Status:                      typedef.SegmentLapStatusInvalid,
			Manufacturer:                typedef.ManufacturerInvalid,
			Sport:                       typedef.SportInvalid,
			SubSport:                    typedef.SubSportInvalid,
			Event:                       typedef.EventInvalid,
			EventType:                   typedef.EventTypeInvalid,
			EventGroup:                  basetype.Uint8Invalid,
			SportEvent:                  typedef.SportEventInvalid,
			StartPositionLat:            basetype.Sint32Invalid,
			StartPositionLong:           basetype.Sint32Invalid,
			EndPositionLat:              basetype.Sint32Invalid,
			EndPositionLong:             basetype.Sint32Invalid,
			NecLat:                      basetype.Sint32Invalid,
			NecLong:                     basetype.Sint32Invalid,
			SwcLat:                      basetype.Sint32Invalid,
			SwcLong:                     basetype.Sint32Invalid,
			TotalElapsedTime:            basetype.Uint32Invalid,
			TotalTimerTime:              basetype.Uint32Invalid,
			TotalMovingTime:             basetype.Uint32Invalid,
			ActiveTime:                  basetype.Uint32Invalid,
			TotalDistance:               basetype.Uint32Invalid,
			TotalAscent:                 basetype.Uint16Invalid,
			TotalDescent:                basetype.Uint16Invalid,
			TotalCycles:                 basetype.Uint32Invalid,
			TotalWork:                   basetype.Uint32Invalid,
			EnhancedAvgAltitude:         basetype.Uint32Invalid,
			EnhancedMaxAltitude:         basetype.Uint32Invalid,
			EnhancedMinAltitude:         basetype.Uint32Invalid,
			TotalFractionalAscent:       basetype.Uint8Invalid,
			TotalFractionalDescent:      basetype.Uint8Invalid,
			AvgSpeed:                    basetype.Uint16Invalid,
			MaxSpeed:                    basetype.Uint16Invalid,
			AvgHeartRate:                basetype.Uint8Invalid,
			MaxHeartRate:                basetype.Uint8Invalid,
			MinHeartRate:                basetype.Uint8Invalid,
			AvgCadence:                  basetype.Uint8Invalid,
			MaxCadence:                  basetype.Uint8Invalid,
			AvgFractionalCadence:        basetype.Uint8Invalid,
			MaxFractionalCadence:        basetype.Uint8Invalid,
			TotalFractionalCycles:       basetype.Uint8Invalid,
			AvgPower:                    basetype.Uint16Invalid,
			MaxPower:                    basetype.Uint16Invalid,
			NormalizedPower:             basetype.Uint16Invalid,
			LeftRightBalance:            typedef.LeftRightBalance100Invalid,
			AvgPosVerticalSpeed:         basetype.Sint16Invalid,
			AvgNegVerticalSpeed:         basetype.Sint16Invalid,
			MaxPosVerticalSpeed:         basetype.Sint16Invalid,
			MaxNegVerticalSpeed:         basetype.Sint16Invalid,
			AvgTemperature:              basetype.Sint8Invalid,
			MaxTemperature:              basetype.Sint8Invalid,
			GpsAccuracy:                 basetype.Uint8Invalid,
			TotalCalories:               basetype.Uint16Invalid,
			TotalFatCalories:            basetype.Uint16Invalid,
			AvgLeftPedalSmoothness:      basetype.Uint8Invalid,
			AvgRightPedalSmoothness:     basetype.Uint8Invalid,
			AvgCombinedPedalSmoothness:  basetype.Uint8Invalid,
			AvgLeftPco:                  basetype.Sint8Invalid,
			AvgRightPco:                 basetype.Sint8Invalid,
			TimeStanding:                basetype.Uint32Invalid,
			StandCount:                  basetype.Uint16Invalid,
			FrontGearShiftCount:         basetype.Uint16Invalid,
			RearGearShiftCount:          basetype.Uint16Invalid,
			RepetitionNum:               basetype.Uint16Invalid,
			WktStepIndex:                typedef.MessageIndexInvalid,
		},
	}

	result := ExtractSegmentLaps(laps)
	if len(result) != 1 {
		t.Fatalf("expected 1 segment lap, got %d", len(result))
	}

	rec := result[0]

	// Name
	if rec.Name == nil || *rec.Name != "Test Segment" {
		t.Errorf("Name: got %v, want 'Test Segment'", rec.Name)
	}

	// UUID
	if rec.UUID == nil || *rec.UUID != "abc-123" {
		t.Errorf("UUID: got %v, want 'abc-123'", rec.UUID)
	}

	// AvgGrade: 500 / 100.0 = 5.0
	if rec.AvgGradePct == nil || *rec.AvgGradePct != 5.0 {
		t.Errorf("AvgGradePct: got %v, want 5.0", rec.AvgGradePct)
	}

	// AvgPosGrade: 800 / 100.0 = 8.0
	if rec.AvgPosGradePct == nil || *rec.AvgPosGradePct != 8.0 {
		t.Errorf("AvgPosGradePct: got %v, want 8.0", rec.AvgPosGradePct)
	}

	// AvgLeftTorqueEffectiveness: 180 / 2.0 = 90.0
	if rec.AvgLeftTorqueEffectivenessPct == nil || *rec.AvgLeftTorqueEffectivenessPct != 90.0 {
		t.Errorf("AvgLeftTorqueEffectivenessPct: got %v, want 90.0", rec.AvgLeftTorqueEffectivenessPct)
	}

	// AvgRightTorqueEffectiveness should be nil
	if rec.AvgRightTorqueEffectivenessPct != nil {
		t.Errorf("AvgRightTorqueEffectivenessPct: expected nil")
	}

	// Power Phase: [0, 180] / 0.7111111
	if rec.AvgLeftPowerPhaseStartDeg == nil {
		t.Fatal("AvgLeftPowerPhaseStartDeg: expected non-nil")
	}
	if math.Abs(*rec.AvgLeftPowerPhaseStartDeg-0.0) > 0.01 {
		t.Errorf("AvgLeftPowerPhaseStartDeg: got %f, want 0.0", *rec.AvgLeftPowerPhaseStartDeg)
	}
	if rec.AvgLeftPowerPhaseEndDeg == nil {
		t.Fatal("AvgLeftPowerPhaseEndDeg: expected non-nil")
	}
	// 180 / 0.7111111 ≈ 253.125
	want := 180.0 / 0.7111111
	if math.Abs(*rec.AvgLeftPowerPhaseEndDeg-want) > 0.01 {
		t.Errorf("AvgLeftPowerPhaseEndDeg: got %f, want %f", *rec.AvgLeftPowerPhaseEndDeg, want)
	}

	// Peak phase should be nil
	if rec.AvgLeftPowerPhasePeakStartDeg != nil {
		t.Errorf("AvgLeftPowerPhasePeakStartDeg: expected nil")
	}
}

func TestExtractSegmentLaps_TimeAndDistance(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	laps := []*mesgdef.SegmentLap{
		{
			Timestamp:                   now,
			MessageIndex:                typedef.MessageIndex(0),
			TotalElapsedTime:            300000,                             // /1000 = 300.0s
			TotalTimerTime:              295000,                             // /1000 = 295.0s
			TotalDistance:               500000,                             // /100 = 5000.0m
			TotalAscent:                 150,
			TotalDescent:                80,
			EnhancedAvgAltitude:         7500,                               // /5 - 500 = 1000.0m
			AvgSpeed:                    5000,                               // /1000 = 5.0 m/s
			AvgHeartRate:                145,
			AvgCadence:                  90,
			AvgPower:                    250,

			// Other fields invalid
			Name:                        "",
			Uuid:                        "",
			Status:                      typedef.SegmentLapStatusInvalid,
			Manufacturer:                typedef.ManufacturerInvalid,
			Sport:                       typedef.SportInvalid,
			SubSport:                    typedef.SubSportInvalid,
			Event:                       typedef.EventInvalid,
			EventType:                   typedef.EventTypeInvalid,
			EventGroup:                  basetype.Uint8Invalid,
			SportEvent:                  typedef.SportEventInvalid,
			StartPositionLat:            basetype.Sint32Invalid,
			StartPositionLong:           basetype.Sint32Invalid,
			EndPositionLat:              basetype.Sint32Invalid,
			EndPositionLong:             basetype.Sint32Invalid,
			NecLat:                      basetype.Sint32Invalid,
			NecLong:                     basetype.Sint32Invalid,
			SwcLat:                      basetype.Sint32Invalid,
			SwcLong:                     basetype.Sint32Invalid,
			TotalMovingTime:             basetype.Uint32Invalid,
			ActiveTime:                  basetype.Uint32Invalid,
			TotalCycles:                 basetype.Uint32Invalid,
			TotalWork:                   basetype.Uint32Invalid,
			EnhancedMaxAltitude:         basetype.Uint32Invalid,
			EnhancedMinAltitude:         basetype.Uint32Invalid,
			TotalFractionalAscent:       basetype.Uint8Invalid,
			TotalFractionalDescent:      basetype.Uint8Invalid,
			MaxSpeed:                    basetype.Uint16Invalid,
			MaxHeartRate:                basetype.Uint8Invalid,
			MinHeartRate:                basetype.Uint8Invalid,
			MaxCadence:                  basetype.Uint8Invalid,
			AvgFractionalCadence:        basetype.Uint8Invalid,
			MaxFractionalCadence:        basetype.Uint8Invalid,
			TotalFractionalCycles:       basetype.Uint8Invalid,
			MaxPower:                    basetype.Uint16Invalid,
			NormalizedPower:             basetype.Uint16Invalid,
			LeftRightBalance:            typedef.LeftRightBalance100Invalid,
			AvgGrade:                    basetype.Sint16Invalid,
			AvgPosGrade:                 basetype.Sint16Invalid,
			AvgNegGrade:                 basetype.Sint16Invalid,
			MaxPosGrade:                 basetype.Sint16Invalid,
			MaxNegGrade:                 basetype.Sint16Invalid,
			AvgPosVerticalSpeed:         basetype.Sint16Invalid,
			AvgNegVerticalSpeed:         basetype.Sint16Invalid,
			MaxPosVerticalSpeed:         basetype.Sint16Invalid,
			MaxNegVerticalSpeed:         basetype.Sint16Invalid,
			AvgTemperature:              basetype.Sint8Invalid,
			MaxTemperature:              basetype.Sint8Invalid,
			GpsAccuracy:                 basetype.Uint8Invalid,
			TotalCalories:               basetype.Uint16Invalid,
			TotalFatCalories:            basetype.Uint16Invalid,
			AvgLeftTorqueEffectiveness:  basetype.Uint8Invalid,
			AvgRightTorqueEffectiveness: basetype.Uint8Invalid,
			AvgLeftPedalSmoothness:      basetype.Uint8Invalid,
			AvgRightPedalSmoothness:     basetype.Uint8Invalid,
			AvgCombinedPedalSmoothness:  basetype.Uint8Invalid,
			AvgLeftPco:                  basetype.Sint8Invalid,
			AvgRightPco:                 basetype.Sint8Invalid,
			TimeStanding:                basetype.Uint32Invalid,
			StandCount:                  basetype.Uint16Invalid,
			FrontGearShiftCount:         basetype.Uint16Invalid,
			RearGearShiftCount:          basetype.Uint16Invalid,
			RepetitionNum:               basetype.Uint16Invalid,
			WktStepIndex:                typedef.MessageIndexInvalid,
		},
	}

	result := ExtractSegmentLaps(laps)
	rec := result[0]

	if rec.TotalElapsedTimeS == nil || *rec.TotalElapsedTimeS != 300.0 {
		t.Errorf("TotalElapsedTimeS: got %v, want 300.0", rec.TotalElapsedTimeS)
	}
	if rec.TotalDistanceM == nil || *rec.TotalDistanceM != 5000.0 {
		t.Errorf("TotalDistanceM: got %v, want 5000.0", rec.TotalDistanceM)
	}
	if rec.TotalAscentM == nil || *rec.TotalAscentM != 150 {
		t.Errorf("TotalAscentM: got %v, want 150", rec.TotalAscentM)
	}
	if rec.AvgAltitudeM == nil || *rec.AvgAltitudeM != 1000.0 {
		t.Errorf("AvgAltitudeM: got %v, want 1000.0", rec.AvgAltitudeM)
	}
	if rec.AvgSpeedMs == nil || *rec.AvgSpeedMs != 5.0 {
		t.Errorf("AvgSpeedMs: got %v, want 5.0", rec.AvgSpeedMs)
	}
	if rec.AvgHeartRate == nil || *rec.AvgHeartRate != 145 {
		t.Errorf("AvgHeartRate: got %v, want 145", rec.AvgHeartRate)
	}
	if rec.AvgCadence == nil || *rec.AvgCadence != 90 {
		t.Errorf("AvgCadence: got %v, want 90", rec.AvgCadence)
	}
	if rec.AvgPowerW == nil || *rec.AvgPowerW != 250 {
		t.Errorf("AvgPowerW: got %v, want 250", rec.AvgPowerW)
	}
}
