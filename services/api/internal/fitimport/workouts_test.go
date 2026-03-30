package fitimport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

func TestExtractWorkout_SourceHash(t *testing.T) {
	workout := &mesgdef.Workout{
		WktName:        "Test",
		WktDescription: "A test workout",
		Sport:          typedef.Sport(1),    // cycling
		SubSport:       typedef.SubSport(0), // generic
		NumValidSteps:  3,
		Capabilities:   typedef.WorkoutCapabilitiesInvalid, // skip
		PoolLength:     basetype.Uint16Invalid,             // skip
		PoolLengthUnit: typedef.DisplayMeasureInvalid,      // skip
	}

	result := ExtractWorkout(workout)
	if result == nil {
		t.Fatal("expected non-nil WorkoutRecord")
	}

	// Name
	if result.Name == nil || *result.Name != "Test" {
		t.Errorf("Name: got %v, want 'Test'", result.Name)
	}

	// Description
	if result.Description == nil || *result.Description != "A test workout" {
		t.Errorf("Description: got %v, want 'A test workout'", result.Description)
	}

	// Sport
	if result.Sport == nil || *result.Sport != 1 {
		t.Errorf("Sport: got %v, want 1", result.Sport)
	}

	// SubSport
	if result.SubSport == nil || *result.SubSport != 0 {
		t.Errorf("SubSport: got %v, want 0", result.SubSport)
	}

	// NumValidSteps
	if result.NumValidSteps == nil || *result.NumValidSteps != 3 {
		t.Errorf("NumValidSteps: got %v, want 3", result.NumValidSteps)
	}

	// SourceHash: SHA256 of "Test|1|0|3"
	expected := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%d|%d", "Test", 1, 0, 3))
	expectedHash := hex.EncodeToString(expected[:])
	if result.SourceHash != expectedHash {
		t.Errorf("SourceHash:\ngot  %s\nwant %s", result.SourceHash, expectedHash)
	}

	// Capabilities should be nil (invalid = 0x0)
	if result.Capabilities != nil {
		t.Errorf("Capabilities: expected nil, got %v", *result.Capabilities)
	}

	// PoolLength should be nil
	if result.PoolLengthM != nil {
		t.Errorf("PoolLengthM: expected nil, got %v", *result.PoolLengthM)
	}
}

func TestExtractWorkout_Nil(t *testing.T) {
	result := ExtractWorkout(nil)
	if result != nil {
		t.Errorf("expected nil for nil workout, got %v", result)
	}
}

func TestExtractWorkoutSteps_ExerciseWeight(t *testing.T) {
	steps := []*mesgdef.WorkoutStep{
		{
			MessageIndex:                   typedef.MessageIndex(0),
			WktStepName:                    "Bench Press",
			Notes:                          "Heavy set",
			DurationType:                   typedef.WktStepDuration(0),
			DurationValue:                  10,
			TargetType:                     typedef.WktStepTargetInvalid,
			TargetValue:                    basetype.Uint32Invalid,
			CustomTargetValueLow:           basetype.Uint32Invalid,
			CustomTargetValueHigh:          basetype.Uint32Invalid,
			Intensity:                      typedef.IntensityInvalid,
			ExerciseCategory:               typedef.ExerciseCategory(4), // bench_press
			ExerciseName:                   100,
			ExerciseWeight:                 5000, // /100 = 50.0 kg (NOT /16!)
			WeightDisplayUnit:              typedef.FitBaseUnitInvalid,
			Equipment:                      typedef.WorkoutEquipmentInvalid,
			SecondaryTargetType:            typedef.WktStepTargetInvalid,
			SecondaryTargetValue:           basetype.Uint32Invalid,
			SecondaryCustomTargetValueLow:  basetype.Uint32Invalid,
			SecondaryCustomTargetValueHigh: basetype.Uint32Invalid,
		},
	}

	result := ExtractWorkoutSteps(steps)
	if len(result) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result))
	}

	rec := result[0]

	// StepName
	if rec.StepName == nil || *rec.StepName != "Bench Press" {
		t.Errorf("StepName: got %v, want 'Bench Press'", rec.StepName)
	}

	// Notes
	if rec.Notes == nil || *rec.Notes != "Heavy set" {
		t.Errorf("Notes: got %v, want 'Heavy set'", rec.Notes)
	}

	// ExerciseWeight: 5000 / 100.0 = 50.0 (NOT /16 like Set.Weight!)
	if rec.ExerciseWeightKg == nil || *rec.ExerciseWeightKg != 50.0 {
		t.Errorf("ExerciseWeightKg: got %v, want 50.0 (scale /100, not /16)", rec.ExerciseWeightKg)
	}

	// ExerciseCategory
	if rec.ExerciseCategory == nil || *rec.ExerciseCategory != 4 {
		t.Errorf("ExerciseCategory: got %v, want 4", rec.ExerciseCategory)
	}

	// ExerciseName
	if rec.ExerciseName == nil || *rec.ExerciseName != 100 {
		t.Errorf("ExerciseName: got %v, want 100", rec.ExerciseName)
	}

	// DurationType
	if rec.DurationType == nil || *rec.DurationType != 0 {
		t.Errorf("DurationType: got %v, want 0", rec.DurationType)
	}

	// DurationValue
	if rec.DurationValue == nil || *rec.DurationValue != 10 {
		t.Errorf("DurationValue: got %v, want 10", rec.DurationValue)
	}
}

func TestExtractWorkoutSteps_EmptyInput(t *testing.T) {
	result := ExtractWorkoutSteps(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 steps for nil input, got %d", len(result))
	}
}
