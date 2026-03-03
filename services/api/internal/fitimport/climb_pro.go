package fitimport

import (
	"strings"

	"github.com/muktihari/fit/kit/semicircles"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractClimbProEvents(events []*mesgdef.ClimbPro) []model.ClimbProEventRecord {
	var result []model.ClimbProEventRecord

	for _, e := range events {
		rec := model.ClimbProEventRecord{
			Timestamp: e.Timestamp,
		}

		// Event type
		if e.ClimbProEvent != typedef.ClimbProEventInvalid {
			rec.EventType = strings.ToLower(e.ClimbProEvent.String())
		}

		// Climb number
		if e.ClimbNumber != basetype.Uint16Invalid {
			rec.ClimbNumber = int(e.ClimbNumber)
		}

		// Climb category
		if e.ClimbCategory != basetype.Uint8Invalid {
			rec.ClimbCategory = int(e.ClimbCategory)
		}

		// Current distance (float32, already in meters)
		if !isNaNFloat32(e.CurrentDist) {
			rec.CurrentDistM = float64(e.CurrentDist)
		}

		// GPS (semicircles → degrees)
		if e.PositionLat != basetype.Sint32Invalid {
			rec.Lat = semicircles.ToDegrees(e.PositionLat)
		}
		if e.PositionLong != basetype.Sint32Invalid {
			rec.Lng = semicircles.ToDegrees(e.PositionLong)
		}

		result = append(result, rec)
	}

	return result
}

func isNaNFloat32(f float32) bool {
	return f != f
}
