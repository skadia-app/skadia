package fitimport

import (
	"strings"

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

		result = append(result, rec)
	}

	return result
}
