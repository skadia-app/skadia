package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) InsertActivityJumps(ctx context.Context, activityID string, jumps []model.JumpRecord) error {
	if len(jumps) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, j := range jumps {
		batch.Queue(`
			INSERT INTO activity_jumps (
				activity_id, timestamp, distance_m, height_m,
				hang_time_s, score, rotations, speed_ms,
				lat, lng
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7, $8,
				$9, $10
			)`,
			activityID, j.Timestamp, j.DistanceM, j.HeightM,
			j.HangTimeS, j.Score, j.Rotations, j.SpeedMs,
			j.Lat, j.Lng,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range jumps {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert activity jump: %w", err)
		}
	}
	return nil
}

func (db *DB) DeleteActivityJumps(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_jumps WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete activity jumps: %w", err)
	}
	return nil
}

func (db *DB) GetActivityJumps(ctx context.Context, activityID string) ([]model.JumpRecord, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT timestamp, distance_m, height_m,
			hang_time_s, score, rotations, speed_ms,
			lat, lng
		FROM activity_jumps
		WHERE activity_id = $1
		ORDER BY timestamp`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get activity jumps: %w", err)
	}
	defer rows.Close()

	var result []model.JumpRecord
	for rows.Next() {
		var r model.JumpRecord
		if err := rows.Scan(
			&r.Timestamp, &r.DistanceM, &r.HeightM,
			&r.HangTimeS, &r.Score, &r.Rotations, &r.SpeedMs,
			&r.Lat, &r.Lng,
		); err != nil {
			return nil, fmt.Errorf("scan activity jump: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity jumps: %w", err)
	}
	if result == nil {
		result = []model.JumpRecord{}
	}
	return result, nil
}
