package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) InsertActivitySets(ctx context.Context, activityID string, sets []model.SetRecord) error {
	if len(sets) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, s := range sets {
		batch.Queue(`
			INSERT INTO activity_sets (
				activity_id, message_index, timestamp, start_time,
				duration_s, set_type, repetitions, weight_kg,
				weight_display_unit, exercise_categories, exercise_subtypes,
				wkt_step_index
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7, $8,
				$9, $10, $11,
				$12
			)`,
			activityID, s.MessageIndex, s.Timestamp, s.StartTime,
			s.DurationS, s.SetType, s.Repetitions, s.WeightKg,
			s.WeightDisplayUnit, s.ExerciseCategories, s.ExerciseSubtypes,
			s.WktStepIndex,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range sets {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert activity set: %w", err)
		}
	}
	return nil
}

func (db *DB) DeleteActivitySets(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_sets WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete activity sets: %w", err)
	}
	return nil
}

func (db *DB) GetActivitySets(ctx context.Context, activityID string, limit, offset int) ([]model.SetRecord, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT message_index, timestamp, start_time,
			duration_s, set_type, repetitions, weight_kg,
			weight_display_unit, exercise_categories, exercise_subtypes,
			wkt_step_index
		FROM activity_sets
		WHERE activity_id = $1
		ORDER BY message_index
		LIMIT $2 OFFSET $3`,
		activityID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get activity sets: %w", err)
	}
	defer rows.Close()

	var result []model.SetRecord
	for rows.Next() {
		var r model.SetRecord
		if err := rows.Scan(
			&r.MessageIndex, &r.Timestamp, &r.StartTime,
			&r.DurationS, &r.SetType, &r.Repetitions, &r.WeightKg,
			&r.WeightDisplayUnit, &r.ExerciseCategories, &r.ExerciseSubtypes,
			&r.WktStepIndex,
		); err != nil {
			return nil, fmt.Errorf("scan activity set: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity sets: %w", err)
	}
	if result == nil {
		result = []model.SetRecord{}
	}
	return result, nil
}
