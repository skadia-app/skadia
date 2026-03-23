package fitimport

import (
	"math"

	"github.com/muktihari/fit/kit/semicircles"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractJumps(jumps []*mesgdef.Jump) []model.JumpRecord {
	var result []model.JumpRecord

	for _, j := range jumps {
		rec := model.JumpRecord{
			Timestamp: j.Timestamp,
		}

		// Distance (float32 → float64)
		if math.Float32bits(j.Distance) != basetype.Float32Invalid {
			v := float64(j.Distance)
			rec.DistanceM = &v
		}

		// Height (float32 → float64)
		if math.Float32bits(j.Height) != basetype.Float32Invalid {
			v := float64(j.Height)
			rec.HeightM = &v
		}

		// HangTime (float32 → float64)
		if math.Float32bits(j.HangTime) != basetype.Float32Invalid {
			v := float64(j.HangTime)
			rec.HangTimeS = &v
		}

		// Score (float32 → float64)
		if math.Float32bits(j.Score) != basetype.Float32Invalid {
			v := float64(j.Score)
			rec.Score = &v
		}

		// Rotations
		if j.Rotations != basetype.Uint8Invalid {
			v := int16(j.Rotations)
			rec.Rotations = &v
		}

		// Speed: prefer EnhancedSpeed (Scale: 1000), fallback Speed (Scale: 1000)
		if j.EnhancedSpeed != basetype.Uint32Invalid {
			v := float64(j.EnhancedSpeed) / 1000.0
			rec.SpeedMs = &v
		} else if j.Speed != basetype.Uint16Invalid {
			v := float64(j.Speed) / 1000.0
			rec.SpeedMs = &v
		}

		// GPS (semicircles → degrees)
		if j.PositionLat != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(j.PositionLat)
			rec.Lat = &v
		}
		if j.PositionLong != basetype.Sint32Invalid {
			v := semicircles.ToDegrees(j.PositionLong)
			rec.Lng = &v
		}

		result = append(result, rec)
	}

	return result
}
