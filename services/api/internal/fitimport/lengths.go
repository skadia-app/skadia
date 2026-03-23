package fitimport

import (
	"strings"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractLengths(lengths []*mesgdef.Length) []model.LengthRecord {
	var result []model.LengthRecord

	for _, l := range lengths {
		rec := model.LengthRecord{
			MessageIndex: int(l.MessageIndex),
			StartTime:    l.StartTime,
			EndTime:      l.Timestamp,
		}

		// Time (uint32, Scale: 1000 → seconds)
		if l.TotalElapsedTime != basetype.Uint32Invalid {
			rec.TotalElapsedTimeS = float64(l.TotalElapsedTime) / 1000.0
		}
		if l.TotalTimerTime != basetype.Uint32Invalid {
			rec.TotalTimerTimeS = float64(l.TotalTimerTime) / 1000.0
		}

		// Strokes
		if l.TotalStrokes != basetype.Uint16Invalid {
			rec.TotalStrokes = int(l.TotalStrokes)
		}

		// Calories
		if l.TotalCalories != basetype.Uint16Invalid {
			rec.TotalCalories = int(l.TotalCalories)
		}

		// Speed (uint16, Scale: 1000 → m/s)
		if l.AvgSpeed != basetype.Uint16Invalid {
			rec.AvgSpeedMS = float64(l.AvgSpeed) / 1000.0
		}

		// Swimming cadence
		if l.AvgSwimmingCadence != basetype.Uint8Invalid {
			rec.AvgSwimmingCadence = int(l.AvgSwimmingCadence)
		}

		// Swim stroke type
		if l.SwimStroke != typedef.SwimStrokeInvalid {
			rec.SwimStroke = strings.ToLower(l.SwimStroke.String())
		}

		// Length type
		if l.LengthType != typedef.LengthTypeInvalid {
			rec.LengthType = strings.ToLower(l.LengthType.String())
		}

		// Stroke count per stroke type ([]uint16 → []int16)
		if l.StrokeCount != nil {
			arr := make([]int16, len(l.StrokeCount))
			for j, s := range l.StrokeCount {
				arr[j] = int16(s)
			}
			rec.StrokeCount = arr
		}

		result = append(result, rec)
	}

	return result
}
