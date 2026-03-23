package fitimport

import (
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractSets(sets []*mesgdef.Set) []model.SetRecord {
	var result []model.SetRecord

	for _, s := range sets {
		rec := model.SetRecord{
			MessageIndex: int16(s.MessageIndex),
		}

		// Timestamp
		if !s.Timestamp.IsZero() {
			ts := s.Timestamp
			rec.Timestamp = &ts
		}

		// StartTime
		if !s.StartTime.IsZero() {
			ts := s.StartTime
			rec.StartTime = &ts
		}

		// Duration (Scale: 1000 → seconds)
		if s.Duration != basetype.Uint32Invalid {
			v := float64(s.Duration) / 1000.0
			rec.DurationS = &v
		}

		// SetType
		if s.SetType != typedef.SetTypeInvalid {
			v := int16(s.SetType)
			rec.SetType = &v
		}

		// Repetitions
		if s.Repetitions != basetype.Uint16Invalid {
			v := int32(s.Repetitions)
			rec.Repetitions = &v
		}

		// Weight (Scale: 16 → kg)
		if s.Weight != basetype.Uint16Invalid {
			v := float64(s.Weight) / 16.0
			rec.WeightKg = &v
		}

		// WeightDisplayUnit
		if s.WeightDisplayUnit != typedef.FitBaseUnitInvalid {
			v := int16(s.WeightDisplayUnit)
			rec.WeightDisplayUnit = &v
		}

		// ExerciseCategories
		if len(s.Category) > 0 {
			cats := make([]int16, len(s.Category))
			for i, c := range s.Category {
				cats[i] = int16(c)
			}
			rec.ExerciseCategories = cats
		}

		// ExerciseSubtypes
		if len(s.CategorySubtype) > 0 {
			subs := make([]int16, len(s.CategorySubtype))
			for i, v := range s.CategorySubtype {
				subs[i] = int16(v)
			}
			rec.ExerciseSubtypes = subs
		}

		// WktStepIndex
		if s.WktStepIndex != typedef.MessageIndexInvalid {
			v := int16(s.WktStepIndex)
			rec.WktStepIndex = &v
		}

		result = append(result, rec)
	}

	return result
}
