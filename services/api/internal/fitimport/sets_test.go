package fitimport

import (
	"testing"
	"time"

	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
)

func TestExtractSets_WeightAndRepetitions(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	sets := []*mesgdef.Set{
		{
			Timestamp:         now,
			StartTime:         now.Add(-30 * time.Second),
			Duration:          5000,                                                                         // /1000 = 5.0s
			SetType:           typedef.SetType(0),                                                           // active
			Repetitions:       10,                                                                           // int32(10)
			Weight:            1600,                                                                         // /16 = 100.0 kg
			WeightDisplayUnit: typedef.FitBaseUnitInvalid,                                                   // skip
			Category:          []typedef.ExerciseCategory{typedef.ExerciseCategory(3), typedef.ExerciseCategory(7)}, // bench_press=3, deadlift=7
			CategorySubtype:   []uint16{1, 2},
			MessageIndex:      typedef.MessageIndex(0),
			WktStepIndex:      typedef.MessageIndexInvalid, // skip
		},
	}

	result := ExtractSets(sets)
	if len(result) != 1 {
		t.Fatalf("expected 1 set, got %d", len(result))
	}

	rec := result[0]

	// Weight: 1600 / 16.0 = 100.0
	if rec.WeightKg == nil || *rec.WeightKg != 100.0 {
		t.Errorf("WeightKg: got %v, want 100.0", rec.WeightKg)
	}

	// Repetitions
	if rec.Repetitions == nil || *rec.Repetitions != 10 {
		t.Errorf("Repetitions: got %v, want 10", rec.Repetitions)
	}

	// Duration: 5000 / 1000.0 = 5.0
	if rec.DurationS == nil || *rec.DurationS != 5.0 {
		t.Errorf("DurationS: got %v, want 5.0", rec.DurationS)
	}

	// SetType
	if rec.SetType == nil || *rec.SetType != 0 {
		t.Errorf("SetType: got %v, want 0", rec.SetType)
	}

	// ExerciseCategories
	if len(rec.ExerciseCategories) != 2 {
		t.Fatalf("ExerciseCategories: got %d, want 2", len(rec.ExerciseCategories))
	}
	if rec.ExerciseCategories[0] != 3 || rec.ExerciseCategories[1] != 7 {
		t.Errorf("ExerciseCategories: got %v, want [3 7]", rec.ExerciseCategories)
	}

	// ExerciseSubtypes
	if len(rec.ExerciseSubtypes) != 2 {
		t.Fatalf("ExerciseSubtypes: got %d, want 2", len(rec.ExerciseSubtypes))
	}
	if rec.ExerciseSubtypes[0] != 1 || rec.ExerciseSubtypes[1] != 2 {
		t.Errorf("ExerciseSubtypes: got %v, want [1 2]", rec.ExerciseSubtypes)
	}

	// Timestamps
	if rec.Timestamp == nil || !rec.Timestamp.Equal(now) {
		t.Errorf("Timestamp: got %v, want %v", rec.Timestamp, now)
	}
	if rec.StartTime == nil || !rec.StartTime.Equal(now.Add(-30*time.Second)) {
		t.Errorf("StartTime mismatch")
	}

	// WeightDisplayUnit should be nil (invalid)
	if rec.WeightDisplayUnit != nil {
		t.Errorf("WeightDisplayUnit: expected nil, got %v", *rec.WeightDisplayUnit)
	}

	// WktStepIndex should be nil (invalid)
	if rec.WktStepIndex != nil {
		t.Errorf("WktStepIndex: expected nil, got %v", *rec.WktStepIndex)
	}
}

func TestExtractSets_EmptyInput(t *testing.T) {
	result := ExtractSets(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 sets for nil input, got %d", len(result))
	}
}
