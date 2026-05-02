package garmin

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/fitimport"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// parsedWorkoutData holds the pure decode output of a workout FIT file.
type parsedWorkoutData struct {
	Workout *model.WorkoutRecord
	Steps   []model.WorkoutStepRecord
}

// decodeWorkout decodes a raw workout FIT payload into typed records without any I/O.
func decodeWorkout(raw []byte) (parsedWorkoutData, error) {
	dec := decoder.New(bytes.NewReader(raw), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return parsedWorkoutData{}, fmt.Errorf("decode workout FIT: %w", err)
	}

	var workout *mesgdef.Workout
	var steps []*mesgdef.WorkoutStep
	for i := range fit.Messages {
		msg := &fit.Messages[i]
		switch msg.Num {
		case typedef.MesgNumWorkout:
			workout = mesgdef.NewWorkout(msg)
		case typedef.MesgNumWorkoutStep:
			steps = append(steps, mesgdef.NewWorkoutStep(msg))
		}
	}

	rec := fitimport.ExtractWorkout(workout)
	stepRecs := fitimport.ExtractWorkoutSteps(steps)

	return parsedWorkoutData{
		Workout: rec,
		Steps:   stepRecs,
	}, nil
}

func processWorkoutFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	wTS := time.Now().UTC()
	if _, err := storeHealthFIT(ctx, db, fs, data, userID, "workout", fmt.Sprintf("workout_%s.fit", wTS.Format("20060102_150405")), wTS); err != nil {
		slog.Warn("processWorkoutFIT: storeHealthFIT failed", "error", err)
	}

	parsed, err := decodeWorkout(data)
	if err != nil {
		return err
	}

	if parsed.Workout == nil {
		return nil
	}

	_, err = db.UpsertWorkout(ctx, userID, *parsed.Workout, parsed.Steps)
	return err
}
