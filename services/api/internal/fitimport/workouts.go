package fitimport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractWorkout(workout *mesgdef.Workout) *model.WorkoutRecord {
	if workout == nil {
		return nil
	}

	rec := model.WorkoutRecord{}

	// Name
	if workout.WktName != basetype.StringInvalid && workout.WktName != "" {
		rec.Name = &workout.WktName
	}

	// Description
	if workout.WktDescription != basetype.StringInvalid && workout.WktDescription != "" {
		rec.Description = &workout.WktDescription
	}

	// Sport
	if workout.Sport != typedef.SportInvalid {
		v := int16(workout.Sport)
		rec.Sport = &v
	}

	// SubSport
	if workout.SubSport != typedef.SubSportInvalid {
		v := int16(workout.SubSport)
		rec.SubSport = &v
	}

	// Capabilities (uint32z bitmask → int64)
	if workout.Capabilities != typedef.WorkoutCapabilitiesInvalid {
		v := int64(workout.Capabilities)
		rec.Capabilities = &v
	}

	// NumValidSteps
	if workout.NumValidSteps != basetype.Uint16Invalid {
		v := int16(workout.NumValidSteps)
		rec.NumValidSteps = &v
	}

	// PoolLength (Scale: 100 → meters)
	if workout.PoolLength != basetype.Uint16Invalid {
		v := float64(workout.PoolLength) / 100.0
		rec.PoolLengthM = &v
	}

	// PoolLengthUnit
	if workout.PoolLengthUnit != typedef.DisplayMeasureInvalid {
		v := workout.PoolLengthUnit.String()
		rec.PoolLengthUnit = &v
	}

	// SourceHash: SHA256 of key fields
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%d",
		workout.WktName, workout.Sport, workout.SubSport, workout.NumValidSteps)))
	rec.SourceHash = hex.EncodeToString(hash[:])

	return &rec
}

func ExtractWorkoutSteps(steps []*mesgdef.WorkoutStep) []model.WorkoutStepRecord {
	var result []model.WorkoutStepRecord

	for _, s := range steps {
		rec := model.WorkoutStepRecord{
			MessageIndex: int16(s.MessageIndex),
		}

		// StepName
		if s.WktStepName != basetype.StringInvalid && s.WktStepName != "" {
			rec.StepName = &s.WktStepName
		}

		// Notes
		if s.Notes != basetype.StringInvalid && s.Notes != "" {
			rec.Notes = &s.Notes
		}

		// DurationType
		if s.DurationType != typedef.WktStepDurationInvalid {
			v := int16(s.DurationType)
			rec.DurationType = &v
		}

		// DurationValue
		if s.DurationValue != basetype.Uint32Invalid {
			v := int32(s.DurationValue)
			rec.DurationValue = &v
		}

		// TargetType
		if s.TargetType != typedef.WktStepTargetInvalid {
			v := int16(s.TargetType)
			rec.TargetType = &v
		}

		// TargetValue
		if s.TargetValue != basetype.Uint32Invalid {
			v := int32(s.TargetValue)
			rec.TargetValue = &v
		}

		// CustomTargetLow
		if s.CustomTargetValueLow != basetype.Uint32Invalid {
			v := int32(s.CustomTargetValueLow)
			rec.CustomTargetLow = &v
		}

		// CustomTargetHigh
		if s.CustomTargetValueHigh != basetype.Uint32Invalid {
			v := int32(s.CustomTargetValueHigh)
			rec.CustomTargetHigh = &v
		}

		// Intensity
		if s.Intensity != typedef.IntensityInvalid {
			v := int16(s.Intensity)
			rec.Intensity = &v
		}

		// ExerciseCategory
		if s.ExerciseCategory != typedef.ExerciseCategoryInvalid {
			v := int16(s.ExerciseCategory)
			rec.ExerciseCategory = &v
		}

		// ExerciseName
		if s.ExerciseName != basetype.Uint16Invalid {
			v := int16(s.ExerciseName)
			rec.ExerciseName = &v
		}

		// ExerciseWeight (Scale: 100 → kg)
		if s.ExerciseWeight != basetype.Uint16Invalid {
			v := float64(s.ExerciseWeight) / 100.0
			rec.ExerciseWeightKg = &v
		}

		// WeightDisplayUnit
		if s.WeightDisplayUnit != typedef.FitBaseUnitInvalid {
			v := int16(s.WeightDisplayUnit)
			rec.WeightDisplayUnit = &v
		}

		// Equipment
		if s.Equipment != typedef.WorkoutEquipmentInvalid {
			v := int16(s.Equipment)
			rec.Equipment = &v
		}

		// SecondaryTargetType
		if s.SecondaryTargetType != typedef.WktStepTargetInvalid {
			v := int16(s.SecondaryTargetType)
			rec.SecondaryTargetType = &v
		}

		// SecondaryTargetValue
		if s.SecondaryTargetValue != basetype.Uint32Invalid {
			v := int32(s.SecondaryTargetValue)
			rec.SecondaryTargetValue = &v
		}

		// SecondaryCustomTargetLow
		if s.SecondaryCustomTargetValueLow != basetype.Uint32Invalid {
			v := int32(s.SecondaryCustomTargetValueLow)
			rec.SecondaryCustomTargetLow = &v
		}

		// SecondaryCustomTargetHigh
		if s.SecondaryCustomTargetValueHigh != basetype.Uint32Invalid {
			v := int32(s.SecondaryCustomTargetValueHigh)
			rec.SecondaryCustomTargetHigh = &v
		}

		result = append(result, rec)
	}

	return result
}
