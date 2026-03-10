package database

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

// GetTimeInZones returns the pre-computed zone distribution for an activity.
func (db *DB) GetTimeInZones(ctx context.Context, activityID string) ([]model.TimeInZone, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT zone_type, zone_index, zone_high, seconds
		 FROM activity_time_in_zone
		 WHERE activity_id = $1
		 ORDER BY zone_type, zone_index`,
		activityID,
	)
	if err != nil {
		return nil, fmt.Errorf("get time in zones: %w", err)
	}
	defer rows.Close()
	var zones []model.TimeInZone
	for rows.Next() {
		var z model.TimeInZone
		if err := rows.Scan(&z.ZoneType, &z.ZoneIndex, &z.ZoneHigh, &z.Seconds); err != nil {
			return nil, fmt.Errorf("scan time in zone: %w", err)
		}
		zones = append(zones, z)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get time in zones: %w", err)
	}
	return zones, nil
}

// SaveTimeInZones upserts time-in-zone data for an activity using a batch for efficiency.
func (db *DB) SaveTimeInZones(ctx context.Context, activityID string, zones []model.TimeInZoneData) error {
	if len(zones) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, z := range zones {
		var zoneHigh *float64
		if z.ZoneHigh > 0 {
			zoneHigh = &z.ZoneHigh
		}
		batch.Queue(
			`INSERT INTO activity_time_in_zone (activity_id, zone_type, zone_index, zone_high, seconds)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (activity_id, zone_type, zone_index) DO UPDATE
			 SET zone_high = EXCLUDED.zone_high, seconds = EXCLUDED.seconds`,
			activityID, z.ZoneType, z.ZoneIndex, zoneHigh, z.Seconds,
		)
	}
	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for range zones {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("save time in zone: %w", err)
		}
	}
	return nil
}

// GetPowerStreamsForPeriod loads power streams for all activities in a date range.
// Returns a map of activityID to ordered power values (one per stream point).
// sportType is optional; pass an empty string to include all sport types.
func (db *DB) GetPowerStreamsForPeriod(ctx context.Context, userID string, from, to time.Time, sportType string) (map[string][]int16, error) {
	var rows pgx.Rows
	var err error
	if sportType != "" {
		rows, err = db.Pool.Query(ctx,
			`SELECT a.id, s.power_w
			 FROM activities a
			 JOIN activity_streams s ON s.activity_id = a.id
			 WHERE a.user_id = $1
			   AND a.started_at >= $2 AND a.started_at <= $3
			   AND a.sport_type = $4
			   AND s.power_w IS NOT NULL AND s.power_w > 0
			 ORDER BY a.id, s.time`,
			userID, from, to, sportType,
		)
	} else {
		rows, err = db.Pool.Query(ctx,
			`SELECT a.id, s.power_w
			 FROM activities a
			 JOIN activity_streams s ON s.activity_id = a.id
			 WHERE a.user_id = $1
			   AND a.started_at >= $2 AND a.started_at <= $3
			   AND s.power_w IS NOT NULL AND s.power_w > 0
			 ORDER BY a.id, s.time`,
			userID, from, to,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("get power streams: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]int16)
	for rows.Next() {
		var actID string
		var power int16
		if err := rows.Scan(&actID, &power); err != nil {
			return nil, fmt.Errorf("scan power stream: %w", err)
		}
		result[actID] = append(result[actID], power)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get power streams: %w", err)
	}
	return result, nil
}

// GetActivityStreams loads all stream points for an activity as StreamPoint slices.
// This is an alias for GetStreamPoints for use in export/zone handlers.
func (db *DB) GetActivityStreams(ctx context.Context, activityID string) ([]model.StreamPoint, error) {
	return db.GetStreamPoints(ctx, activityID)
}

// SportTypeStatRow holds a sport_type and its activity count for a user.
type SportTypeStatRow struct {
	SportType string `json:"sport_type"`
	Count     int    `json:"count"`
}

// GetSportTypeStats returns the activity count per sport_type for a user,
// ordered by count descending.
func (db *DB) GetSportTypeStats(ctx context.Context, userID string) ([]SportTypeStatRow, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT sport_type, COUNT(*)::int AS count
		 FROM activities
		 WHERE user_id = $1
		 GROUP BY sport_type
		 ORDER BY count DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get sport type stats: %w", err)
	}
	defer rows.Close()
	var result []SportTypeStatRow
	for rows.Next() {
		var row SportTypeStatRow
		if err := rows.Scan(&row.SportType, &row.Count); err != nil {
			return nil, fmt.Errorf("scan sport type stat: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get sport type stats: %w", err)
	}
	return result, nil
}
