package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// GetActivityIntervals returns all detected training intervals for an activity.
func (db *DB) GetActivityIntervals(ctx context.Context, activityID string) ([]model.ActivityInterval, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, segment_num, start_time, end_time, duration_s,
		        avg_power_w, avg_heart_rate, avg_speed_ms, avg_cadence, intensity
		 FROM activity_intervals WHERE activity_id = $1
		 ORDER BY segment_num`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get activity intervals: %w", err)
	}
	defer rows.Close()

	var intervals []model.ActivityInterval
	for rows.Next() {
		var iv model.ActivityInterval
		iv.ActivityID = activityID
		if err := rows.Scan(
			&iv.ID, &iv.SegmentNum, &iv.StartTime, &iv.EndTime, &iv.DurationS,
			&iv.AvgPowerW, &iv.AvgHeartRate, &iv.AvgSpeedMS, &iv.AvgCadence, &iv.Intensity,
		); err != nil {
			return nil, fmt.Errorf("scan activity interval: %w", err)
		}
		intervals = append(intervals, iv)
	}
	if intervals == nil {
		intervals = []model.ActivityInterval{}
	}
	return intervals, rows.Err()
}
