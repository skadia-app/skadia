package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) UpsertWorkout(ctx context.Context, userID string, workout model.WorkoutRecord, steps []model.WorkoutStepRecord) (string, error) {
	if workout.SourceHash == "" {
		workout.SourceHash = "empty"
	}

	var workoutID string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO workouts (
			user_id, name, description, sport, sub_sport,
			capabilities, num_valid_steps, pool_length_m, pool_length_unit,
			source_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, source_hash) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			sport = EXCLUDED.sport,
			sub_sport = EXCLUDED.sub_sport,
			capabilities = EXCLUDED.capabilities,
			num_valid_steps = EXCLUDED.num_valid_steps,
			pool_length_m = EXCLUDED.pool_length_m,
			pool_length_unit = EXCLUDED.pool_length_unit
		RETURNING id`,
		userID, workout.Name, workout.Description, workout.Sport, workout.SubSport,
		workout.Capabilities, workout.NumValidSteps, workout.PoolLengthM, workout.PoolLengthUnit,
		workout.SourceHash,
	).Scan(&workoutID)
	if err != nil {
		return "", fmt.Errorf("upsert workout: %w", err)
	}

	// Delete old steps before inserting new ones (prevents UNIQUE violation on re-import)
	_, err = db.Pool.Exec(ctx, `DELETE FROM workout_steps WHERE workout_id = $1`, workoutID)
	if err != nil {
		return "", fmt.Errorf("delete workout steps: %w", err)
	}

	if len(steps) > 0 {
		batch := &pgx.Batch{}
		for _, s := range steps {
			batch.Queue(`
				INSERT INTO workout_steps (
					workout_id, message_index, step_name, notes,
					duration_type, duration_value, target_type, target_value,
					custom_target_low, custom_target_high,
					intensity, exercise_category, exercise_name,
					exercise_weight_kg, weight_display_unit, equipment,
					secondary_target_type, secondary_target_value,
					secondary_custom_target_low, secondary_custom_target_high
				) VALUES (
					$1, $2, $3, $4,
					$5, $6, $7, $8,
					$9, $10,
					$11, $12, $13,
					$14, $15, $16,
					$17, $18,
					$19, $20
				)`,
				workoutID, s.MessageIndex, s.StepName, s.Notes,
				s.DurationType, s.DurationValue, s.TargetType, s.TargetValue,
				s.CustomTargetLow, s.CustomTargetHigh,
				s.Intensity, s.ExerciseCategory, s.ExerciseName,
				s.ExerciseWeightKg, s.WeightDisplayUnit, s.Equipment,
				s.SecondaryTargetType, s.SecondaryTargetValue,
				s.SecondaryCustomTargetLow, s.SecondaryCustomTargetHigh,
			)
		}

		br := db.Pool.SendBatch(ctx, batch)
		defer br.Close()

		for range steps {
			if _, err := br.Exec(); err != nil {
				return "", fmt.Errorf("insert workout step: %w", err)
			}
		}
	}

	return workoutID, nil
}

func (db *DB) GetWorkout(ctx context.Context, workoutID, userID string) (*model.WorkoutRecord, []model.WorkoutStepRecord, []model.WorkoutActivityRef, error) {
	var w model.WorkoutRecord
	err := db.Pool.QueryRow(ctx, `
		SELECT id, name, description, sport, sub_sport,
			capabilities, num_valid_steps, pool_length_m, pool_length_unit,
			source_hash
		FROM workouts
		WHERE id = $1 AND user_id = $2`,
		workoutID, userID,
	).Scan(
		&w.ID, &w.Name, &w.Description, &w.Sport, &w.SubSport,
		&w.Capabilities, &w.NumValidSteps, &w.PoolLengthM, &w.PoolLengthUnit,
		&w.SourceHash,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get workout: %w", err)
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT message_index, step_name, notes,
			duration_type, duration_value, target_type, target_value,
			custom_target_low, custom_target_high,
			intensity, exercise_category, exercise_name,
			exercise_weight_kg, weight_display_unit, equipment,
			secondary_target_type, secondary_target_value,
			secondary_custom_target_low, secondary_custom_target_high
		FROM workout_steps
		WHERE workout_id = $1
		ORDER BY message_index`,
		workoutID,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get workout steps: %w", err)
	}
	defer rows.Close()

	var steps []model.WorkoutStepRecord
	for rows.Next() {
		var s model.WorkoutStepRecord
		if err := rows.Scan(
			&s.MessageIndex, &s.StepName, &s.Notes,
			&s.DurationType, &s.DurationValue, &s.TargetType, &s.TargetValue,
			&s.CustomTargetLow, &s.CustomTargetHigh,
			&s.Intensity, &s.ExerciseCategory, &s.ExerciseName,
			&s.ExerciseWeightKg, &s.WeightDisplayUnit, &s.Equipment,
			&s.SecondaryTargetType, &s.SecondaryTargetValue,
			&s.SecondaryCustomTargetLow, &s.SecondaryCustomTargetHigh,
		); err != nil {
			return nil, nil, nil, fmt.Errorf("scan workout step: %w", err)
		}
		steps = append(steps, s)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("get workout steps: %w", err)
	}
	if steps == nil {
		steps = []model.WorkoutStepRecord{}
	}

	actRows, err := db.Pool.Query(ctx, `
		SELECT id, COALESCE(title, title_auto), started_at, sport_type, distance_m, elapsed_time_s
		FROM activities
		WHERE workout_id = $1 AND user_id = $2
		ORDER BY started_at DESC`,
		workoutID, userID,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get workout activities: %w", err)
	}
	defer actRows.Close()

	var activities []model.WorkoutActivityRef
	for actRows.Next() {
		var a model.WorkoutActivityRef
		if err := actRows.Scan(&a.ID, &a.Title, &a.StartedAt, &a.SportType, &a.DistanceM, &a.ElapsedTimeS); err != nil {
			return nil, nil, nil, fmt.Errorf("scan workout activity: %w", err)
		}
		activities = append(activities, a)
	}
	if err := actRows.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("get workout activities: %w", err)
	}
	if activities == nil {
		activities = []model.WorkoutActivityRef{}
	}

	return &w, steps, activities, nil
}

func (db *DB) ListWorkouts(ctx context.Context, userID string, limit, offset int) ([]model.WorkoutRecord, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT w.id, w.name, w.description, w.sport, w.sub_sport,
			w.capabilities, w.num_valid_steps, w.pool_length_m, w.pool_length_unit,
			w.source_hash,
			COUNT(a.id) AS activity_count
		FROM workouts w
		LEFT JOIN activities a ON a.workout_id = w.id AND a.user_id = w.user_id
		WHERE w.user_id = $1
		GROUP BY w.id
		ORDER BY w.created_at DESC
		LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list workouts: %w", err)
	}
	defer rows.Close()

	var result []model.WorkoutRecord
	for rows.Next() {
		var w model.WorkoutRecord
		if err := rows.Scan(
			&w.ID, &w.Name, &w.Description, &w.Sport, &w.SubSport,
			&w.Capabilities, &w.NumValidSteps, &w.PoolLengthM, &w.PoolLengthUnit,
			&w.SourceHash, &w.ActivityCount,
		); err != nil {
			return nil, fmt.Errorf("scan workout: %w", err)
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list workouts: %w", err)
	}
	if result == nil {
		result = []model.WorkoutRecord{}
	}
	return result, nil
}

func (db *DB) SetActivityWorkoutID(ctx context.Context, activityID, workoutID string) error {
	_, err := db.Pool.Exec(ctx, `UPDATE activities SET workout_id = $2 WHERE id = $1`, activityID, workoutID)
	if err != nil {
		return fmt.Errorf("set activity workout id: %w", err)
	}
	return nil
}
