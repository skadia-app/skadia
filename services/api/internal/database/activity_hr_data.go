package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) InsertActivityHrData(ctx context.Context, activityID string, hrs []model.HrDataRecord) error {
	if len(hrs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, h := range hrs {
		batch.Queue(`
			INSERT INTO activity_hr_data (
				activity_id, timestamp, fractional_timestamp,
				filtered_bpm, event_timestamp
			) VALUES ($1, $2, $3, $4, $5)`,
			activityID, h.Timestamp, h.FractionalTimestamp,
			h.FilteredBpm, h.EventTimestamp,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range hrs {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert activity hr data: %w", err)
		}
	}
	return nil
}

func (db *DB) DeleteActivityHrData(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_hr_data WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete activity hr data: %w", err)
	}
	return nil
}

func (db *DB) GetActivityHrData(ctx context.Context, activityID string) ([]model.HrDataRecord, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT timestamp, fractional_timestamp,
			filtered_bpm, event_timestamp
		FROM activity_hr_data
		WHERE activity_id = $1
		ORDER BY timestamp`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get activity hr data: %w", err)
	}
	defer rows.Close()

	var result []model.HrDataRecord
	for rows.Next() {
		var r model.HrDataRecord
		if err := rows.Scan(
			&r.Timestamp, &r.FractionalTimestamp,
			&r.FilteredBpm, &r.EventTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan activity hr data: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity hr data: %w", err)
	}
	if result == nil {
		result = []model.HrDataRecord{}
	}
	return result, nil
}
