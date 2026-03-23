package fitimport

import (
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/muktihari/fit/kit/semicircles"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractLaps(laps []*mesgdef.Lap) []model.LapRecord {
	var result []model.LapRecord

	for i, l := range laps {
		rec := model.LapRecord{
			ID:           uuid.New().String(),
			MessageIndex: i,
			StartTime:    l.StartTime,
			EndTime:      l.Timestamp,
		}

		// Lap trigger
		if l.LapTrigger != typedef.LapTriggerInvalid {
			rec.LapTrigger = strings.ToLower(l.LapTrigger.String())
		}

		// Sport / SubSport
		if l.Sport != typedef.SportInvalid {
			rec.Sport = strings.ToLower(l.Sport.String())
		}
		if l.SubSport != typedef.SubSportInvalid {
			rec.SubSport = strings.ToLower(l.SubSport.String())
		}

		// Time fields (uint32, Scale: 1000 → seconds)
		if l.TotalElapsedTime != basetype.Uint32Invalid {
			rec.TotalElapsedTimeS = float64(l.TotalElapsedTime) / 1000.0
		}
		if l.TotalTimerTime != basetype.Uint32Invalid {
			rec.TotalTimerTimeS = float64(l.TotalTimerTime) / 1000.0
		}
		if l.TotalMovingTime != basetype.Uint32Invalid {
			rec.TotalMovingTimeS = float64(l.TotalMovingTime) / 1000.0
		}

		// Distance (uint32, Scale: 100 → meters)
		if l.TotalDistance != basetype.Uint32Invalid {
			rec.TotalDistanceM = float64(l.TotalDistance) / 100.0
		}

		// Elevation
		if l.TotalAscent != basetype.Uint16Invalid {
			rec.TotalAscentM = int(l.TotalAscent)
		}
		if l.TotalDescent != basetype.Uint16Invalid {
			rec.TotalDescentM = int(l.TotalDescent)
		}

		// Calories
		if l.TotalCalories != basetype.Uint16Invalid {
			rec.TotalCalories = int(l.TotalCalories)
		}

		// Speed (uint32, Scale: 1000 → m/s) - prefer enhanced
		if l.EnhancedAvgSpeed != basetype.Uint32Invalid {
			rec.AvgSpeedMS = float64(l.EnhancedAvgSpeed) / 1000.0
		} else if l.AvgSpeed != basetype.Uint16Invalid {
			rec.AvgSpeedMS = float64(l.AvgSpeed) / 1000.0
		}
		if l.EnhancedMaxSpeed != basetype.Uint32Invalid {
			rec.MaxSpeedMS = float64(l.EnhancedMaxSpeed) / 1000.0
		} else if l.MaxSpeed != basetype.Uint16Invalid {
			rec.MaxSpeedMS = float64(l.MaxSpeed) / 1000.0
		}

		// Heart rate
		if l.AvgHeartRate != basetype.Uint8Invalid {
			rec.AvgHeartRate = int(l.AvgHeartRate)
		}
		if l.MaxHeartRate != basetype.Uint8Invalid {
			rec.MaxHeartRate = int(l.MaxHeartRate)
		}
		if l.MinHeartRate != basetype.Uint8Invalid {
			rec.MinHeartRate = int(l.MinHeartRate)
		}

		// Cadence
		if l.AvgCadence != basetype.Uint8Invalid {
			rec.AvgCadence = int(l.AvgCadence)
		}
		if l.MaxCadence != basetype.Uint8Invalid {
			rec.MaxCadence = int(l.MaxCadence)
		}

		// Power
		if l.AvgPower != basetype.Uint16Invalid {
			rec.AvgPowerW = int(l.AvgPower)
		}
		if l.MaxPower != basetype.Uint16Invalid {
			rec.MaxPowerW = int(l.MaxPower)
		}
		if l.NormalizedPower != basetype.Uint16Invalid {
			rec.NormalizedPowerW = int(l.NormalizedPower)
		}

		// Temperature
		if l.AvgTemperature != basetype.Sint8Invalid {
			rec.AvgTemperatureC = int(l.AvgTemperature)
		}

		// GPS (semicircles → degrees)
		if l.StartPositionLat != basetype.Sint32Invalid {
			rec.StartLat = semicircles.ToDegrees(l.StartPositionLat)
		}
		if l.StartPositionLong != basetype.Sint32Invalid {
			rec.StartLng = semicircles.ToDegrees(l.StartPositionLong)
		}
		if l.EndPositionLat != basetype.Sint32Invalid {
			rec.EndLat = semicircles.ToDegrees(l.EndPositionLat)
		}
		if l.EndPositionLong != basetype.Sint32Invalid {
			rec.EndLng = semicircles.ToDegrees(l.EndPositionLong)
		}

		// Work
		if l.TotalWork != basetype.Uint32Invalid {
			rec.TotalWorkJ = int(l.TotalWork)
		}

		// Intensity
		if l.Intensity != typedef.IntensityInvalid {
			rec.Intensity = strings.ToLower(l.Intensity.String())
		}

		// Swim
		if l.SwimStroke != typedef.SwimStrokeInvalid {
			rec.SwimStroke = strings.ToLower(l.SwimStroke.String())
		}
		if l.NumLengths != basetype.Uint16Invalid {
			rec.NumLengths = int(l.NumLengths)
		}
		if l.NumActiveLengths != basetype.Uint16Invalid {
			rec.NumActiveLengths = int(l.NumActiveLengths)
		}

		// Altitude (prefer enhanced: uint32, Scale: 5, Offset: 500)
		if l.EnhancedAvgAltitude != basetype.Uint32Invalid {
			rec.AvgAltitudeM = float64(l.EnhancedAvgAltitude)/5.0 - 500.0
		}
		if l.EnhancedMaxAltitude != basetype.Uint32Invalid {
			rec.MaxAltitudeM = float64(l.EnhancedMaxAltitude)/5.0 - 500.0
		}
		if l.EnhancedMinAltitude != basetype.Uint32Invalid {
			rec.MinAltitudeM = float64(l.EnhancedMinAltitude)/5.0 - 500.0
		}

		// --- FIT SDK Extended Fields ---

		// Cycles/Strokes
		if l.TotalCycles != basetype.Uint32Invalid {
			v := int32(l.TotalCycles)
			rec.TotalCycles = &v
		}
		if l.TotalFatCalories != basetype.Uint16Invalid {
			v := int32(l.TotalFatCalories)
			rec.TotalFatCalories = &v
		}
		if l.AvgStrokeDistance != basetype.Uint16Invalid {
			v := float64(l.AvgStrokeDistance) / 100.0
			rec.AvgStrokeDistanceM = &v
		}
		if l.FirstLengthIndex != basetype.Uint16Invalid {
			v := int32(l.FirstLengthIndex)
			rec.FirstLengthIndex = &v
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
		if l.StrokeCount != nil {
			arr := make([]int32, len(l.StrokeCount))
			for j, s := range l.StrokeCount {
				arr[j] = int32(s)
			}
			rec.StrokeCount = arr
		}
		if l.ZoneCount != nil {
			arr := make([]int32, len(l.ZoneCount))
			for j, z := range l.ZoneCount {
				arr[j] = int32(z)
			}
			rec.ZoneCount = arr
		}

		// Running Dynamics
		if l.AvgVerticalOscillation != basetype.Uint16Invalid {
			v := float64(l.AvgVerticalOscillation) / 10.0
			rec.AvgVerticalOscillationMm = &v
		}
		if l.AvgStanceTime != basetype.Uint16Invalid {
			v := float64(l.AvgStanceTime) / 10.0
			rec.AvgStanceTimeMs = &v
		}
		if l.AvgStanceTimePercent != basetype.Uint16Invalid {
			v := float64(l.AvgStanceTimePercent) / 100.0
			rec.AvgStanceTimePct = &v
		}
		if l.AvgStanceTimeBalance != basetype.Uint16Invalid {
			v := float64(l.AvgStanceTimeBalance) / 100.0
			rec.AvgStanceTimeBalancePct = &v
		}
		if l.AvgStepLength != basetype.Uint16Invalid {
			v := float64(l.AvgStepLength) / 10.0
			rec.AvgStepLengthMm = &v
		}
		if l.AvgVerticalRatio != basetype.Uint16Invalid {
			v := float64(l.AvgVerticalRatio) / 100.0
			rec.AvgVerticalRatioPct = &v
		}

		// Grade (int16, Scale: 100 → %)
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

		// Vertical Speed (int16, Scale: 1000 → m/s)
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
		if l.AvgVam != basetype.Uint16Invalid {
			v := float64(l.AvgVam) / 1000.0
			rec.AvgVamMs = &v
		}

		// Temperature Extended
		if l.MaxTemperature != basetype.Sint8Invalid {
			v := int16(l.MaxTemperature)
			rec.MaxTemperatureC = &v
		}
		if l.MinTemperature != basetype.Sint8Invalid {
			v := int16(l.MinTemperature)
			rec.MinTemperatureC = &v
		}
		if l.AvgCoreTemperature != basetype.Uint16Invalid {
			v := float64(l.AvgCoreTemperature) / 100.0
			rec.AvgCoreTemperatureC = &v
		}
		if l.MinCoreTemperature != basetype.Uint16Invalid {
			v := float64(l.MinCoreTemperature) / 100.0
			rec.MinCoreTemperatureC = &v
		}
		if l.MaxCoreTemperature != basetype.Uint16Invalid {
			v := float64(l.MaxCoreTemperature) / 100.0
			rec.MaxCoreTemperatureC = &v
		}

		// Pedal Analytics (uint8, Scale: 2 → /2.0)
		if l.AvgLeftTorqueEffectiveness != basetype.Uint8Invalid {
			v := float64(l.AvgLeftTorqueEffectiveness) / 2.0
			rec.AvgLeftTorqueEffectivenessPct = &v
		}
		if l.AvgRightTorqueEffectiveness != basetype.Uint8Invalid {
			v := float64(l.AvgRightTorqueEffectiveness) / 2.0
			rec.AvgRightTorqueEffectivenessPct = &v
		}
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
		if l.AvgLeftPco != basetype.Sint8Invalid {
			v := int16(l.AvgLeftPco)
			rec.AvgLeftPcoMm = &v
		}
		if l.AvgRightPco != basetype.Sint8Invalid {
			v := int16(l.AvgRightPco)
			rec.AvgRightPcoMm = &v
		}

		// Power Phase arrays ([]uint8, Scale: 0.7111111 → degrees)
		// Each array has [start, end] indexes
		if len(l.AvgLeftPowerPhase) >= 2 {
			start := float64(l.AvgLeftPowerPhase[0]) * 0.7111111
			rec.AvgLeftPowerPhaseStartDeg = &start
			end := float64(l.AvgLeftPowerPhase[1]) * 0.7111111
			rec.AvgLeftPowerPhaseEndDeg = &end
		}
		if len(l.AvgLeftPowerPhasePeak) >= 2 {
			start := float64(l.AvgLeftPowerPhasePeak[0]) * 0.7111111
			rec.AvgLeftPowerPhasePeakStartDeg = &start
			end := float64(l.AvgLeftPowerPhasePeak[1]) * 0.7111111
			rec.AvgLeftPowerPhasePeakEndDeg = &end
		}
		if len(l.AvgRightPowerPhase) >= 2 {
			start := float64(l.AvgRightPowerPhase[0]) * 0.7111111
			rec.AvgRightPowerPhaseStartDeg = &start
			end := float64(l.AvgRightPowerPhase[1]) * 0.7111111
			rec.AvgRightPowerPhaseEndDeg = &end
		}
		if len(l.AvgRightPowerPhasePeak) >= 2 {
			start := float64(l.AvgRightPowerPhasePeak[0]) * 0.7111111
			rec.AvgRightPowerPhasePeakStartDeg = &start
			end := float64(l.AvgRightPowerPhasePeak[1]) * 0.7111111
			rec.AvgRightPowerPhasePeakEndDeg = &end
		}

		// Power/Cadence by Position (arrays)
		if l.AvgPowerPosition != nil {
			arr := make([]int32, len(l.AvgPowerPosition))
			for j, v := range l.AvgPowerPosition {
				arr[j] = int32(v)
			}
			rec.AvgPowerPositionW = arr
		}
		if l.MaxPowerPosition != nil {
			arr := make([]int32, len(l.MaxPowerPosition))
			for j, v := range l.MaxPowerPosition {
				arr[j] = int32(v)
			}
			rec.MaxPowerPositionW = arr
		}
		if l.AvgCadencePosition != nil {
			arr := make([]int16, len(l.AvgCadencePosition))
			for j, v := range l.AvgCadencePosition {
				arr[j] = int16(v)
			}
			rec.AvgCadencePosition = arr
		}
		if l.MaxCadencePosition != nil {
			arr := make([]int16, len(l.MaxCadencePosition))
			for j, v := range l.MaxCadencePosition {
				arr[j] = int16(v)
			}
			rec.MaxCadencePosition = arr
		}

		// Standing/Sitting
		if l.TimeStanding != basetype.Uint32Invalid {
			v := float64(l.TimeStanding) / 1000.0
			rec.TimeStandingS = &v
		}
		if l.StandCount != basetype.Uint16Invalid {
			v := int32(l.StandCount)
			rec.StandCount = &v
		}

		// L/R Balance (LeftRightBalance100: uint16, mask 0x3FFF, scale /100)
		if l.LeftRightBalance != typedef.LeftRightBalance100Invalid {
			v := float64(uint16(l.LeftRightBalance)&0x3FFF) / 100.0
			rec.LeftRightBalance = &v
		}

		// E-Bike
		if l.AvgLevMotorPower != basetype.Uint16Invalid {
			v := int32(l.AvgLevMotorPower)
			rec.AvgLevMotorPowerW = &v
		}
		if l.MaxLevMotorPower != basetype.Uint16Invalid {
			v := int32(l.MaxLevMotorPower)
			rec.MaxLevMotorPowerW = &v
		}
		if l.LevBatteryConsumption != basetype.Uint8Invalid {
			v := float64(l.LevBatteryConsumption) / 2.0
			rec.LevBatteryConsumptionPct = &v
		}

		// Grit/Flow (float32 → float64; invalid = NaN)
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

		// Jumps
		if l.JumpCount != basetype.Uint16Invalid {
			v := int32(l.JumpCount)
			rec.JumpCount = &v
		}

		// Scores
		if l.PlayerScore != basetype.Uint16Invalid {
			v := int32(l.PlayerScore)
			rec.PlayerScore = &v
		}
		if l.OpponentScore != basetype.Uint16Invalid {
			v := int32(l.OpponentScore)
			rec.OpponentScore = &v
		}

		// Hemoglobin ([]uint16 arrays, scaled per element)
		if l.AvgTotalHemoglobinConc != nil {
			arr := make([]float64, len(l.AvgTotalHemoglobinConc))
			for j, v := range l.AvgTotalHemoglobinConc {
				arr[j] = float64(v) / 100.0
			}
			rec.AvgTotalHemoglobinConc = arr
		}
		if l.MinTotalHemoglobinConc != nil {
			arr := make([]float64, len(l.MinTotalHemoglobinConc))
			for j, v := range l.MinTotalHemoglobinConc {
				arr[j] = float64(v) / 100.0
			}
			rec.MinTotalHemoglobinConc = arr
		}
		if l.MaxTotalHemoglobinConc != nil {
			arr := make([]float64, len(l.MaxTotalHemoglobinConc))
			for j, v := range l.MaxTotalHemoglobinConc {
				arr[j] = float64(v) / 100.0
			}
			rec.MaxTotalHemoglobinConc = arr
		}
		if l.AvgSaturatedHemoglobinPercent != nil {
			arr := make([]float64, len(l.AvgSaturatedHemoglobinPercent))
			for j, v := range l.AvgSaturatedHemoglobinPercent {
				arr[j] = float64(v) / 10.0
			}
			rec.AvgSaturatedHemoglobinPct = arr
		}
		if l.MinSaturatedHemoglobinPercent != nil {
			arr := make([]float64, len(l.MinSaturatedHemoglobinPercent))
			for j, v := range l.MinSaturatedHemoglobinPercent {
				arr[j] = float64(v) / 10.0
			}
			rec.MinSaturatedHemoglobinPct = arr
		}
		if l.MaxSaturatedHemoglobinPercent != nil {
			arr := make([]float64, len(l.MaxSaturatedHemoglobinPercent))
			for j, v := range l.MaxSaturatedHemoglobinPercent {
				arr[j] = float64(v) / 10.0
			}
			rec.MaxSaturatedHemoglobinPct = arr
		}

		// Diving (uint32, Scale: 1000 → m)
		if l.AvgDepth != basetype.Uint32Invalid {
			v := float64(l.AvgDepth) / 1000.0
			rec.AvgDepthM = &v
		}
		if l.MaxDepth != basetype.Uint32Invalid {
			v := float64(l.MaxDepth) / 1000.0
			rec.MaxDepthM = &v
		}

		// Respiration (prefer enhanced: uint16/100, fallback: uint8)
		if l.EnhancedAvgRespirationRate != basetype.Uint16Invalid {
			v := float64(l.EnhancedAvgRespirationRate) / 100.0
			rec.AvgRespirationRate = &v
		} else if l.AvgRespirationRate != basetype.Uint8Invalid {
			v := float64(l.AvgRespirationRate)
			rec.AvgRespirationRate = &v
		}
		if l.EnhancedMaxRespirationRate != basetype.Uint16Invalid {
			v := float64(l.EnhancedMaxRespirationRate) / 100.0
			rec.MaxRespirationRate = &v
		} else if l.MaxRespirationRate != basetype.Uint8Invalid {
			v := float64(l.MaxRespirationRate)
			rec.MaxRespirationRate = &v
		}

		// Altitude Extended (uint8, Scale: 100 → m)
		if l.TotalFractionalAscent != basetype.Uint8Invalid {
			v := float64(l.TotalFractionalAscent) / 100.0
			rec.TotalFractionalAscentM = &v
		}
		if l.TotalFractionalDescent != basetype.Uint8Invalid {
			v := float64(l.TotalFractionalDescent) / 100.0
			rec.TotalFractionalDescentM = &v
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
		if l.EventGroup != basetype.Uint8Invalid {
			v := int16(l.EventGroup)
			rec.EventGroup = &v
		}
		if l.GpsAccuracy != basetype.Uint8Invalid {
			v := int16(l.GpsAccuracy)
			rec.GpsAccuracyM = &v
		}

		result = append(result, rec)
	}

	return result
}
