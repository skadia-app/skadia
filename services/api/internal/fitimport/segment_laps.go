package fitimport

import (
	"math"

	"github.com/google/uuid"
	"github.com/muktihari/fit/kit/semicircles"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractSegmentLaps(laps []*mesgdef.SegmentLap) []model.SegmentLapRecord {
	var result []model.SegmentLapRecord

	for _, l := range laps {
		rec := model.SegmentLapRecord{
			ID:           uuid.New().String(),
			MessageIndex: int16(l.MessageIndex),
		}

		// Timestamps
		if !l.Timestamp.IsZero() {
			ts := l.Timestamp
			rec.Timestamp = &ts
		}
		if !l.StartTime.IsZero() {
			ts := l.StartTime
			rec.StartTime = &ts
		}

		// Name
		if l.Name != basetype.StringInvalid && l.Name != "" {
			rec.Name = &l.Name
		}

		// UUID
		if l.Uuid != basetype.StringInvalid && l.Uuid != "" {
			rec.UUID = &l.Uuid
		}

		// Status
		if l.Status != typedef.SegmentLapStatusInvalid {
			v := int16(l.Status)
			rec.Status = &v
		}

		// Manufacturer
		if l.Manufacturer != typedef.ManufacturerInvalid {
			v := int16(l.Manufacturer)
			rec.Manufacturer = &v
		}

		// Sport / SubSport
		if l.Sport != typedef.SportInvalid {
			v := int16(l.Sport)
			rec.Sport = &v
		}
		if l.SubSport != typedef.SubSportInvalid {
			v := int16(l.SubSport)
			rec.SubSport = &v
		}

		// Event / EventType / EventGroup / SportEvent
		if l.Event != typedef.EventInvalid {
			v := int16(l.Event)
			rec.Event = &v
		}
		if l.EventType != typedef.EventTypeInvalid {
			v := int16(l.EventType)
			rec.EventType = &v
		}
		if l.EventGroup != basetype.Uint8Invalid {
			v := int16(l.EventGroup)
			rec.EventGroup = &v
		}
		if l.SportEvent != typedef.SportEventInvalid {
			v := int16(l.SportEvent)
			rec.SportEvent = &v
		}

		// GPS (semicircles → degrees)
		if l.StartPositionLat != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.StartPositionLat)
			rec.StartLat = &v
		}
		if l.StartPositionLong != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.StartPositionLong)
			rec.StartLng = &v
		}
		if l.EndPositionLat != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.EndPositionLat)
			rec.EndLat = &v
		}
		if l.EndPositionLong != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.EndPositionLong)
			rec.EndLng = &v
		}
		if l.NecLat != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.NecLat)
			rec.NecLat = &v
		}
		if l.NecLong != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.NecLong)
			rec.NecLng = &v
		}
		if l.SwcLat != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.SwcLat)
			rec.SwcLat = &v
		}
		if l.SwcLong != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(l.SwcLong)
			rec.SwcLng = &v
		}

		// Time (Scale: 1000 → seconds)
		if l.TotalElapsedTime != basetype.Uint32Invalid {
			v := float64(l.TotalElapsedTime) / 1000.0
			rec.TotalElapsedTimeS = &v
		}
		if l.TotalTimerTime != basetype.Uint32Invalid {
			v := float64(l.TotalTimerTime) / 1000.0
			rec.TotalTimerTimeS = &v
		}
		if l.TotalMovingTime != basetype.Uint32Invalid {
			v := float64(l.TotalMovingTime) / 1000.0
			rec.TotalMovingTimeS = &v
		}
		if l.ActiveTime != basetype.Uint32Invalid {
			v := float64(l.ActiveTime) / 1000.0
			rec.ActiveTimeS = &v
		}

		// Distance (Scale: 100 → meters)
		if l.TotalDistance != basetype.Uint32Invalid {
			v := float64(l.TotalDistance) / 100.0
			rec.TotalDistanceM = &v
		}

		// Ascent / Descent
		if l.TotalAscent != basetype.Uint16Invalid {
			v := int32(l.TotalAscent)
			rec.TotalAscentM = &v
		}
		if l.TotalDescent != basetype.Uint16Invalid {
			v := int32(l.TotalDescent)
			rec.TotalDescentM = &v
		}

		// Cycles / Work
		if l.TotalCycles != basetype.Uint32Invalid {
			v := int32(l.TotalCycles)
			rec.TotalCycles = &v
		}
		if l.TotalWork != basetype.Uint32Invalid {
			v := int32(l.TotalWork)
			rec.TotalWorkJ = &v
		}

		// Altitude (prefer enhanced: Scale 5, Offset 500)
		if l.EnhancedAvgAltitude != basetype.Uint32Invalid {
			v := float64(l.EnhancedAvgAltitude)/5.0 - 500.0
			rec.AvgAltitudeM = &v
		}
		if l.EnhancedMaxAltitude != basetype.Uint32Invalid {
			v := float64(l.EnhancedMaxAltitude)/5.0 - 500.0
			rec.MaxAltitudeM = &v
		}
		if l.EnhancedMinAltitude != basetype.Uint32Invalid {
			v := float64(l.EnhancedMinAltitude)/5.0 - 500.0
			rec.MinAltitudeM = &v
		}

		// Fractional ascent/descent (Scale: 100)
		if l.TotalFractionalAscent != basetype.Uint8Invalid {
			v := float64(l.TotalFractionalAscent) / 100.0
			rec.TotalFractionalAscentM = &v
		}
		if l.TotalFractionalDescent != basetype.Uint8Invalid {
			v := float64(l.TotalFractionalDescent) / 100.0
			rec.TotalFractionalDescentM = &v
		}

		// Speed (Scale: 1000 → m/s)
		if l.AvgSpeed != basetype.Uint16Invalid {
			v := float64(l.AvgSpeed) / 1000.0
			rec.AvgSpeedMs = &v
		}
		if l.MaxSpeed != basetype.Uint16Invalid {
			v := float64(l.MaxSpeed) / 1000.0
			rec.MaxSpeedMs = &v
		}

		// Heart Rate
		if l.AvgHeartRate != basetype.Uint8Invalid {
			v := int16(l.AvgHeartRate)
			rec.AvgHeartRate = &v
		}
		if l.MaxHeartRate != basetype.Uint8Invalid {
			v := int16(l.MaxHeartRate)
			rec.MaxHeartRate = &v
		}
		if l.MinHeartRate != basetype.Uint8Invalid {
			v := int16(l.MinHeartRate)
			rec.MinHeartRate = &v
		}

		// Cadence
		if l.AvgCadence != basetype.Uint8Invalid {
			v := int16(l.AvgCadence)
			rec.AvgCadence = &v
		}
		if l.MaxCadence != basetype.Uint8Invalid {
			v := int16(l.MaxCadence)
			rec.MaxCadence = &v
		}
		if l.AvgFractionalCadence != basetype.Uint8Invalid {
			v := float64(l.AvgFractionalCadence) / 128.0
			rec.AvgFractionalCadence = &v
		}
		if l.MaxFractionalCadence != basetype.Uint8Invalid {
			v := float64(l.MaxFractionalCadence) / 128.0
			rec.MaxFractionalCadence = &v
		}
		if l.TotalFractionalCycles != basetype.Uint8Invalid {
			v := float64(l.TotalFractionalCycles) / 128.0
			rec.TotalFractionalCycles = &v
		}

		// Power
		if l.AvgPower != basetype.Uint16Invalid {
			v := int16(l.AvgPower)
			rec.AvgPowerW = &v
		}
		if l.MaxPower != basetype.Uint16Invalid {
			v := int16(l.MaxPower)
			rec.MaxPowerW = &v
		}
		if l.NormalizedPower != basetype.Uint16Invalid {
			v := int16(l.NormalizedPower)
			rec.NormalizedPowerW = &v
		}

		// Left/Right Balance (Scale: 100)
		if l.LeftRightBalance != typedef.LeftRightBalance100Invalid {
			v := float64(l.LeftRightBalance&typedef.LeftRightBalance100Mask) / 100.0
			rec.LeftRightBalance = &v
		}

		// Grade (Scale: 100 → percent)
		if l.AvgGrade != basetype.Sint16Invalid {
			v := float64(l.AvgGrade) / 100.0
			rec.AvgGradePct = &v
		}
		if l.AvgPosGrade != basetype.Sint16Invalid {
			v := float64(l.AvgPosGrade) / 100.0
			rec.AvgPosGradePct = &v
		}
		if l.AvgNegGrade != basetype.Sint16Invalid {
			v := float64(l.AvgNegGrade) / 100.0
			rec.AvgNegGradePct = &v
		}
		if l.MaxPosGrade != basetype.Sint16Invalid {
			v := float64(l.MaxPosGrade) / 100.0
			rec.MaxPosGradePct = &v
		}
		if l.MaxNegGrade != basetype.Sint16Invalid {
			v := float64(l.MaxNegGrade) / 100.0
			rec.MaxNegGradePct = &v
		}

		// Vertical Speed (Scale: 1000 → m/s)
		if l.AvgPosVerticalSpeed != basetype.Sint16Invalid {
			v := float64(l.AvgPosVerticalSpeed) / 1000.0
			rec.AvgPosVerticalSpeedMs = &v
		}
		if l.AvgNegVerticalSpeed != basetype.Sint16Invalid {
			v := float64(l.AvgNegVerticalSpeed) / 1000.0
			rec.AvgNegVerticalSpeedMs = &v
		}
		if l.MaxPosVerticalSpeed != basetype.Sint16Invalid {
			v := float64(l.MaxPosVerticalSpeed) / 1000.0
			rec.MaxPosVerticalSpeedMs = &v
		}
		if l.MaxNegVerticalSpeed != basetype.Sint16Invalid {
			v := float64(l.MaxNegVerticalSpeed) / 1000.0
			rec.MaxNegVerticalSpeedMs = &v
		}

		// Temperature
		if l.AvgTemperature != basetype.Sint8Invalid {
			v := int16(l.AvgTemperature)
			rec.AvgTemperatureC = &v
		}
		if l.MaxTemperature != basetype.Sint8Invalid {
			v := int16(l.MaxTemperature)
			rec.MaxTemperatureC = &v
		}

		// GPS Accuracy
		if l.GpsAccuracy != basetype.Uint8Invalid {
			v := int16(l.GpsAccuracy)
			rec.GpsAccuracyM = &v
		}

		// Calories
		if l.TotalCalories != basetype.Uint16Invalid {
			v := int32(l.TotalCalories)
			rec.TotalCalories = &v
		}
		if l.TotalFatCalories != basetype.Uint16Invalid {
			v := int32(l.TotalFatCalories)
			rec.TotalFatCalories = &v
		}

		// Pedal Analytics — Torque Effectiveness (Scale: 2 → percent)
		if l.AvgLeftTorqueEffectiveness != basetype.Uint8Invalid {
			v := float64(l.AvgLeftTorqueEffectiveness) / 2.0
			rec.AvgLeftTorqueEffectivenessPct = &v
		}
		if l.AvgRightTorqueEffectiveness != basetype.Uint8Invalid {
			v := float64(l.AvgRightTorqueEffectiveness) / 2.0
			rec.AvgRightTorqueEffectivenessPct = &v
		}

		// Pedal Smoothness (Scale: 2 → percent)
		if l.AvgLeftPedalSmoothness != basetype.Uint8Invalid {
			v := float64(l.AvgLeftPedalSmoothness) / 2.0
			rec.AvgLeftPedalSmoothnessPct = &v
		}
		if l.AvgRightPedalSmoothness != basetype.Uint8Invalid {
			v := float64(l.AvgRightPedalSmoothness) / 2.0
			rec.AvgRightPedalSmoothnessPct = &v
		}
		if l.AvgCombinedPedalSmoothness != basetype.Uint8Invalid {
			v := float64(l.AvgCombinedPedalSmoothness) / 2.0
			rec.AvgCombinedPedalSmoothnessPct = &v
		}

		// PCO (mm)
		if l.AvgLeftPco != basetype.Sint8Invalid {
			v := int16(l.AvgLeftPco)
			rec.AvgLeftPcoMm = &v
		}
		if l.AvgRightPco != basetype.Sint8Invalid {
			v := int16(l.AvgRightPco)
			rec.AvgRightPcoMm = &v
		}

		// Power Phase (Scale: 0.7111111 → degrees)
		extractPowerPhase := func(src []uint8) (start, end, peakStart, peakEnd *float64) {
			if len(src) < 2 {
				return
			}
			s := float64(src[0]) / 0.7111111
			e := float64(src[1]) / 0.7111111
			start = &s
			end = &e
			return
		}

		if len(l.AvgLeftPowerPhase) >= 2 {
			s, e, _, _ := extractPowerPhase(l.AvgLeftPowerPhase)
			rec.AvgLeftPowerPhaseStartDeg = s
			rec.AvgLeftPowerPhaseEndDeg = e
		}
		if len(l.AvgLeftPowerPhasePeak) >= 2 {
			s, e, _, _ := extractPowerPhase(l.AvgLeftPowerPhasePeak)
			rec.AvgLeftPowerPhasePeakStartDeg = s
			rec.AvgLeftPowerPhasePeakEndDeg = e
		}
		if len(l.AvgRightPowerPhase) >= 2 {
			s, e, _, _ := extractPowerPhase(l.AvgRightPowerPhase)
			rec.AvgRightPowerPhaseStartDeg = s
			rec.AvgRightPowerPhaseEndDeg = e
		}
		if len(l.AvgRightPowerPhasePeak) >= 2 {
			s, e, _, _ := extractPowerPhase(l.AvgRightPowerPhasePeak)
			rec.AvgRightPowerPhasePeakStartDeg = s
			rec.AvgRightPowerPhasePeakEndDeg = e
		}

		// Power by Position ([]uint16 → []int32)
		if len(l.AvgPowerPosition) > 0 {
			pp := make([]int32, len(l.AvgPowerPosition))
			for i, v := range l.AvgPowerPosition {
				pp[i] = int32(v)
			}
			rec.AvgPowerPositionW = pp
		}
		if len(l.MaxPowerPosition) > 0 {
			pp := make([]int32, len(l.MaxPowerPosition))
			for i, v := range l.MaxPowerPosition {
				pp[i] = int32(v)
			}
			rec.MaxPowerPositionW = pp
		}

		// Cadence by Position ([]uint8 → []int16)
		if len(l.AvgCadencePosition) > 0 {
			cp := make([]int16, len(l.AvgCadencePosition))
			for i, v := range l.AvgCadencePosition {
				cp[i] = int16(v)
			}
			rec.AvgCadencePosition = cp
		}
		if len(l.MaxCadencePosition) > 0 {
			cp := make([]int16, len(l.MaxCadencePosition))
			for i, v := range l.MaxCadencePosition {
				cp[i] = int16(v)
			}
			rec.MaxCadencePosition = cp
		}

		// Standing / Sitting
		if l.TimeStanding != basetype.Uint32Invalid {
			v := float64(l.TimeStanding) / 1000.0
			rec.TimeStandingS = &v
		}
		if l.StandCount != basetype.Uint16Invalid {
			v := int32(l.StandCount)
			rec.StandCount = &v
		}

		// Gear Shifts
		if l.FrontGearShiftCount != basetype.Uint16Invalid {
			v := int32(l.FrontGearShiftCount)
			rec.FrontGearShiftCount = &v
		}
		if l.RearGearShiftCount != basetype.Uint16Invalid {
			v := int32(l.RearGearShiftCount)
			rec.RearGearShiftCount = &v
		}

		// Grit / Flow (float32 → float64)
		if math.Float32bits(l.TotalGrit) != basetype.Float32Invalid {
			v := float64(l.TotalGrit)
			rec.TotalGrit = &v
		}
		if math.Float32bits(l.TotalFlow) != basetype.Float32Invalid {
			v := float64(l.TotalFlow)
			rec.TotalFlow = &v
		}
		if math.Float32bits(l.AvgGrit) != basetype.Float32Invalid {
			v := float64(l.AvgGrit)
			rec.AvgGrit = &v
		}
		if math.Float32bits(l.AvgFlow) != basetype.Float32Invalid {
			v := float64(l.AvgFlow)
			rec.AvgFlow = &v
		}

		// Misc
		if l.RepetitionNum != basetype.Uint16Invalid {
			v := int32(l.RepetitionNum)
			rec.RepetitionNum = &v
		}
		if l.WktStepIndex != typedef.MessageIndexInvalid {
			v := int16(l.WktStepIndex)
			rec.WktStepIndex = &v
		}

		result = append(result, rec)
	}

	return result
}
