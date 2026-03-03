package fitimport

import (
	"strings"

	"github.com/muktihari/fit/kit/semicircles"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractSplits(splits []*mesgdef.Split) []model.SplitRecord {
	var result []model.SplitRecord

	for _, s := range splits {
		rec := model.SplitRecord{
			MessageIndex: int(s.MessageIndex),
			StartTime:    s.StartTime,
			EndTime:      s.EndTime,
		}

		// Split type
		if s.SplitType != typedef.SplitTypeInvalid {
			rec.SplitType = strings.ToLower(s.SplitType.String())
		}

		// Time (uint32, Scale: 1000 → seconds)
		if s.TotalElapsedTime != basetype.Uint32Invalid {
			rec.TotalElapsedTimeS = float64(s.TotalElapsedTime) / 1000.0
		}
		if s.TotalTimerTime != basetype.Uint32Invalid {
			rec.TotalTimerTimeS = float64(s.TotalTimerTime) / 1000.0
		}
		if s.TotalMovingTime != basetype.Uint32Invalid {
			rec.TotalMovingTimeS = float64(s.TotalMovingTime) / 1000.0
		}

		// Distance (uint32, Scale: 100 → meters)
		if s.TotalDistance != basetype.Uint32Invalid {
			rec.TotalDistanceM = float64(s.TotalDistance) / 100.0
		}

		// Elevation
		if s.TotalAscent != basetype.Uint16Invalid {
			rec.TotalAscentM = int(s.TotalAscent)
		}
		if s.TotalDescent != basetype.Uint16Invalid {
			rec.TotalDescentM = int(s.TotalDescent)
		}

		// Calories (uint32 in split, not uint16)
		if s.TotalCalories != basetype.Uint32Invalid {
			rec.TotalCalories = int(s.TotalCalories)
		}

		// Speed (uint32, Scale: 1000 → m/s)
		if s.AvgSpeed != basetype.Uint32Invalid {
			rec.AvgSpeedMS = float64(s.AvgSpeed) / 1000.0
		}
		if s.MaxSpeed != basetype.Uint32Invalid {
			rec.MaxSpeedMS = float64(s.MaxSpeed) / 1000.0
		}

		// Vertical speed (int32, Scale: 1000 → m/s)
		if s.AvgVertSpeed != basetype.Sint32Invalid {
			rec.AvgVertSpeedMS = float64(s.AvgVertSpeed) / 1000.0
		}

		// GPS (semicircles → degrees)
		if s.StartPositionLat != basetype.Sint32Invalid {
			rec.StartLat = semicircles.ToDegrees(s.StartPositionLat)
		}
		if s.StartPositionLong != basetype.Sint32Invalid {
			rec.StartLng = semicircles.ToDegrees(s.StartPositionLong)
		}
		if s.EndPositionLat != basetype.Sint32Invalid {
			rec.EndLat = semicircles.ToDegrees(s.EndPositionLat)
		}
		if s.EndPositionLong != basetype.Sint32Invalid {
			rec.EndLng = semicircles.ToDegrees(s.EndPositionLong)
		}

		// Start elevation (uint32, Scale: 5, Offset: 500)
		if s.StartElevation != basetype.Uint32Invalid {
			rec.StartElevationM = float64(s.StartElevation)/5.0 - 500.0
		}

		result = append(result, rec)
	}

	return result
}

func ExtractSplitSummaries(summaries []*mesgdef.SplitSummary) []model.SplitSummaryRecord {
	var result []model.SplitSummaryRecord

	for _, s := range summaries {
		rec := model.SplitSummaryRecord{
			MessageIndex: int(s.MessageIndex),
		}

		// Split type
		if s.SplitType != typedef.SplitTypeInvalid {
			rec.SplitType = strings.ToLower(s.SplitType.String())
		}

		// Num splits
		if s.NumSplits != basetype.Uint16Invalid {
			rec.NumSplits = int(s.NumSplits)
		}

		// Time (uint32, Scale: 1000 → seconds)
		if s.TotalTimerTime != basetype.Uint32Invalid {
			rec.TotalTimerTimeS = float64(s.TotalTimerTime) / 1000.0
		}
		if s.TotalMovingTime != basetype.Uint32Invalid {
			rec.TotalMovingTimeS = float64(s.TotalMovingTime) / 1000.0
		}

		// Distance (uint32, Scale: 100 → meters)
		if s.TotalDistance != basetype.Uint32Invalid {
			rec.TotalDistanceM = float64(s.TotalDistance) / 100.0
		}

		// Elevation
		if s.TotalAscent != basetype.Uint16Invalid {
			rec.TotalAscentM = int(s.TotalAscent)
		}
		if s.TotalDescent != basetype.Uint16Invalid {
			rec.TotalDescentM = int(s.TotalDescent)
		}

		// Calories (uint32)
		if s.TotalCalories != basetype.Uint32Invalid {
			rec.TotalCalories = int(s.TotalCalories)
		}

		// Speed (uint32, Scale: 1000 → m/s)
		if s.AvgSpeed != basetype.Uint32Invalid {
			rec.AvgSpeedMS = float64(s.AvgSpeed) / 1000.0
		}
		if s.MaxSpeed != basetype.Uint32Invalid {
			rec.MaxSpeedMS = float64(s.MaxSpeed) / 1000.0
		}

		// Vertical speed (int32, Scale: 1000 → m/s)
		if s.AvgVertSpeed != basetype.Sint32Invalid {
			rec.AvgVertSpeedMS = float64(s.AvgVertSpeed) / 1000.0
		}

		// Heart rate
		if s.AvgHeartRate != basetype.Uint8Invalid {
			rec.AvgHeartRate = int(s.AvgHeartRate)
		}
		if s.MaxHeartRate != basetype.Uint8Invalid {
			rec.MaxHeartRate = int(s.MaxHeartRate)
		}

		result = append(result, rec)
	}

	return result
}
