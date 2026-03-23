package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) InsertActivityEvents(ctx context.Context, activityID string, events []model.EventRecord) error {
	if len(events) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, e := range events {
		batch.Queue(`
			INSERT INTO activity_events (
				activity_id, timestamp, event, event_type, event_group,
				data, front_gear_num, front_gear, rear_gear_num, rear_gear,
				radar_threat_level_max, radar_threat_count,
				radar_threat_avg_approach_speed_ms, radar_threat_max_approach_speed_ms,
				score, opponent_score, device_index, activity_type, start_timestamp
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, $8, $9, $10,
				$11, $12,
				$13, $14,
				$15, $16, $17, $18, $19
			)`,
			activityID, e.Timestamp, e.Event, e.EventType, e.EventGroup,
			e.Data, e.FrontGearNum, e.FrontGear, e.RearGearNum, e.RearGear,
			e.RadarThreatLevelMax, e.RadarThreatCount,
			e.RadarThreatAvgApproachSpeedMs, e.RadarThreatMaxApproachSpeedMs,
			e.Score, e.OpponentScore, e.DeviceIndex, e.ActivityType, e.StartTimestamp,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	for range events {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert activity event: %w", err)
		}
	}
	return nil
}

func (db *DB) DeleteActivityEvents(ctx context.Context, activityID string) error {
	_, err := db.Pool.Exec(ctx, `DELETE FROM activity_events WHERE activity_id = $1`, activityID)
	if err != nil {
		return fmt.Errorf("delete activity events: %w", err)
	}
	return nil
}

func (db *DB) GetActivityEvents(ctx context.Context, activityID string, limit, offset int) ([]model.EventRecord, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT timestamp, event, event_type, event_group,
			data, front_gear_num, front_gear, rear_gear_num, rear_gear,
			radar_threat_level_max, radar_threat_count,
			radar_threat_avg_approach_speed_ms, radar_threat_max_approach_speed_ms,
			score, opponent_score, device_index, activity_type, start_timestamp
		FROM activity_events
		WHERE activity_id = $1
		ORDER BY timestamp
		LIMIT $2 OFFSET $3`,
		activityID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get activity events: %w", err)
	}
	defer rows.Close()

	var result []model.EventRecord
	for rows.Next() {
		var r model.EventRecord
		if err := rows.Scan(
			&r.Timestamp, &r.Event, &r.EventType, &r.EventGroup,
			&r.Data, &r.FrontGearNum, &r.FrontGear, &r.RearGearNum, &r.RearGear,
			&r.RadarThreatLevelMax, &r.RadarThreatCount,
			&r.RadarThreatAvgApproachSpeedMs, &r.RadarThreatMaxApproachSpeedMs,
			&r.Score, &r.OpponentScore, &r.DeviceIndex, &r.ActivityType, &r.StartTimestamp,
		); err != nil {
			return nil, fmt.Errorf("scan activity event: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get activity events: %w", err)
	}
	if result == nil {
		result = []model.EventRecord{}
	}
	return result, nil
}
