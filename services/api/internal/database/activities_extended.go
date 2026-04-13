package database

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/jackc/pgx/v5"
)

// GetTimeInZones returns the pre-computed zone distribution for an activity.
// When both lapID and segmentLapID are nil, only session-level zones are returned
// (rows where lap_id IS NULL AND segment_lap_id IS NULL).
// When lapID is set, zones for that lap are returned.
// When segmentLapID is set, zones for that segment lap are returned.
func (db *DB) GetTimeInZones(ctx context.Context, activityID string, lapID *string, segmentLapID *string) ([]model.TimeInZone, error) {
	var (
		rows pgx.Rows
		err  error
	)
	switch {
	case lapID != nil:
		rows, err = db.Pool.Query(ctx,
			`SELECT zone_type, zone_index, zone_high, seconds, lap_id, segment_lap_id
			 FROM activity_time_in_zone
			 WHERE activity_id = $1 AND lap_id = $2
			 ORDER BY zone_type, zone_index`,
			activityID, *lapID,
		)
	case segmentLapID != nil:
		rows, err = db.Pool.Query(ctx,
			`SELECT zone_type, zone_index, zone_high, seconds, lap_id, segment_lap_id
			 FROM activity_time_in_zone
			 WHERE activity_id = $1 AND segment_lap_id = $2
			 ORDER BY zone_type, zone_index`,
			activityID, *segmentLapID,
		)
	default:
		rows, err = db.Pool.Query(ctx,
			`SELECT zone_type, zone_index, zone_high, seconds, lap_id, segment_lap_id
			 FROM activity_time_in_zone
			 WHERE activity_id = $1 AND lap_id IS NULL AND segment_lap_id IS NULL
			 ORDER BY zone_type, zone_index`,
			activityID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("get time in zones: %w", err)
	}
	defer rows.Close()
	var zones []model.TimeInZone
	for rows.Next() {
		var z model.TimeInZone
		if err := rows.Scan(&z.ZoneType, &z.ZoneIndex, &z.ZoneHigh, &z.Seconds, &z.LapID, &z.SegmentLapID); err != nil {
			return nil, fmt.Errorf("scan time in zone: %w", err)
		}
		zones = append(zones, z)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get time in zones: %w", err)
	}
	return zones, nil
}

// SaveTimeInZones replaces time-in-zone data for a given scope (activity + optional lap/segment_lap).
// It DELETEs existing rows for the scope, then batch-INSERTs the new zones.
// If zones is empty, only the DELETE is performed (clearing old data).
func (db *DB) SaveTimeInZones(ctx context.Context, activityID string, lapID *string, segmentLapID *string, zones []model.TimeInZoneData) error {
	batch := &pgx.Batch{}

	// DELETE existing zones for this scope.
	// IS NOT DISTINCT FROM handles NULL correctly: NULL IS NOT DISTINCT FROM NULL → true.
	batch.Queue(
		`DELETE FROM activity_time_in_zone
		 WHERE activity_id = $1
		 AND (lap_id IS NOT DISTINCT FROM $2)
		 AND (segment_lap_id IS NOT DISTINCT FROM $3)`,
		activityID, lapID, segmentLapID,
	)

	// INSERT new zones.
	for _, z := range zones {
		var zoneHigh *float64
		if z.ZoneHigh > 0 {
			zoneHigh = &z.ZoneHigh
		}
		batch.Queue(
			`INSERT INTO activity_time_in_zone (activity_id, zone_type, zone_index, zone_high, seconds, lap_id, segment_lap_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT DO NOTHING`,
			activityID, z.ZoneType, z.ZoneIndex, zoneHigh, z.Seconds, lapID, segmentLapID,
		)
	}

	br := db.Pool.SendBatch(ctx, batch)
	defer br.Close()

	// Exec DELETE + all INSERTs.
	for range 1 + len(zones) {
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
