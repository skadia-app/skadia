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
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

func processWorkoutFIT(ctx context.Context, db *database.DB, fs shadowfs.ShadowFS, data []byte, userID string) error {
	wTS := time.Now().UTC()
	if _, err := storeHealthFIT(ctx, db, fs, data, userID, "workout", fmt.Sprintf("workout_%s.fit", wTS.Format("20060102_150405")), wTS); err != nil {
		slog.Warn("processWorkoutFIT: storeHealthFIT failed", "error", err)
	}

	dec := decoder.New(bytes.NewReader(data), decoder.WithIgnoreChecksum())
	fit, err := dec.Decode()
	if err != nil {
		return err
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
	if rec == nil {
		return nil
	}
	stepRecs := fitimport.ExtractWorkoutSteps(steps)

	_, err = db.UpsertWorkout(ctx, userID, *rec, stepRecs)
	return err
}
