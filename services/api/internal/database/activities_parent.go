package database

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// InsertMultisportParent inserts a bare parent activity for a multisport session.
// The parent holds no metrics itself; metrics are aggregated from children via
// AggregateParentMetrics after all children are inserted.
func (db *DB) InsertMultisportParent(ctx context.Context, act *model.Activity) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, title, started_at, visibility, enrichment_status)
		VALUES ($1, $2, $3, $4, $5, 'private', 'complete')`,
		act.ID, act.UserID, act.SportType, act.Title, act.StartedAt)
	return err
}

// InsertActivityWithParent inserts an activity and links it to a multisport parent.
// Mirrors InsertActivity but adds parent_id to the INSERT.
func (db *DB) InsertActivityWithParent(ctx context.Context, userID string, data *model.ActivityData, parentID string) (*model.Activity, error) {
	var id string
	var createdAt, updatedAt time.Time
	var isEbike bool

	// Re-use the same column set as InsertActivity, adding parent_id.
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO activities (
			user_id, file_hash, sport_type, sub_type,
			parsed_sport_type, parsed_sub_type,
			fit_sport_id, fit_sub_sport_id,
			started_at, ended_at, elapsed_time_s, moving_time_s,
			distance_m, elevation_gain_m, elevation_loss_m,
			max_elevation_m, min_elevation_m,
			avg_speed_ms, max_speed_ms,
			avg_heart_rate, max_heart_rate,
			avg_power_w, max_power_w, normalized_power_w,
			avg_cadence, avg_temperature_c,
			tss, trimp, is_indoor, has_gps,
			start_location, end_location,
			original_filename, original_file_format,
			source, source_device,
			parent_id
		) VALUES (
			$1, $2, $3, $4,
			$3, $4,
			$5, $6,
			$7, $8, $9, $10,
			$11, $12, $13,
			$14, $15,
			$16, $17,
			$18, $19,
			$20, $21, $22,
			$23, $24,
			$25, $26, $27, $28,
			CASE WHEN $29::float8 != 0 THEN ST_SetSRID(ST_MakePoint($30::float8, $29::float8), 4326) ELSE NULL END,
			CASE WHEN $31::float8 != 0 THEN ST_SetSRID(ST_MakePoint($32::float8, $31::float8), 4326) ELSE NULL END,
			$33, $34,
			$35, $36,
			$37
		)
		RETURNING id, is_ebike, created_at, updated_at`,
		userID, nilIfEmpty(data.FileHash), data.SportType, nilIfEmpty(data.SubType),
		nilIfZeroI16(data.FitSportID), nilIfZeroI16(data.FitSubSportID),
		data.StartedAt, nilIfZeroTime(data.EndedAt), nilIfZeroInt(data.ElapsedTimeS), nilIfZeroInt(data.MovingTimeS),
		nilIfZeroF64(data.DistanceM), nilIfZeroF64(data.ElevationGainM), nilIfZeroF64(data.ElevationLossM),
		nilIfZeroF64(data.MaxElevationM), nilIfZeroF64(data.MinElevationM),
		nilIfZeroF64(data.AvgSpeedMS), nilIfZeroF64(data.MaxSpeedMS),
		nilIfZeroF64(data.AvgHeartRate), nilIfZeroInt(data.MaxHeartRate),
		nilIfZeroF64(data.AvgPowerW), nilIfZeroInt(data.MaxPowerW), nilIfZeroF64(data.NormalizedPowerW),
		nilIfZeroF64(data.AvgCadence), nilIfZeroF64(data.AvgTemperatureC),
		nilIfZeroF64(data.TSS), nilIfZeroF64(data.TRIMP), data.IsIndoor, data.HasGPS,
		data.StartLat, data.StartLng,
		data.EndLat, data.EndLng,
		nilIfEmpty(data.OriginalName), nilIfEmpty(data.FileFormat),
		nilIfEmpty(data.Source), nilIfEmpty(data.SourceDevice),
		parentID,
	).Scan(&id, &isEbike, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert activity with parent: %w", err)
	}

	act := &model.Activity{
		ID:                  id,
		UserID:              userID,
		SportType:           data.SportType,
		SubType:             ptrString(data.SubType),
		Source:              ptrString(data.Source),
		SourceDevice:        ptrString(data.SourceDevice),
		StartedAt:           data.StartedAt,
		IsEbike:             isEbike,
		IsIndoor:            data.IsIndoor,
		HasGPS:              data.HasGPS,
		NeedsClassification: data.SportType == "unknown",
		OriginalFilename:    ptrString(data.OriginalName),
		Visibility:          "private",
		EnrichmentStatus:    "pending",
		ParentID:            &parentID,
	}
	return act, nil
}

// GetChildActivities returns all child activities for a given parent ID.
func (db *DB) GetChildActivities(ctx context.Context, parentID string) ([]model.Activity, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, user_id, sport_type, sub_type, title, title_auto, started_at,
		       elapsed_time_s, moving_time_s, distance_m, elevation_gain_m,
		       elevation_loss_m, avg_heart_rate, avg_power_w, parent_id,
		       is_indoor, has_gps, enrichment_status, visibility,
		       COALESCE(tags, '{}'),
		       file_hash, original_filename, original_file_format
		FROM activities
		WHERE parent_id = $1
		ORDER BY started_at`, parentID)
	if err != nil {
		return nil, fmt.Errorf("get child activities: %w", err)
	}
	defer rows.Close()

	var activities []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.SportType, &a.SubType, &a.Title, &a.TitleAuto, &a.StartedAt,
			&a.ElapsedTimeS, &a.MovingTimeS, &a.DistanceM, &a.ElevationGainM,
			&a.ElevationLossM, &a.AvgHeartRate, &a.AvgPowerW, &a.ParentID,
			&a.IsIndoor, &a.HasGPS, &a.EnrichmentStatus, &a.Visibility,
			&a.Tags,
			&a.FileHash, &a.OriginalFilename, &a.OriginalFileFormat,
		); err != nil {
			return nil, fmt.Errorf("scan child activity: %w", err)
		}
		if a.Tags == nil {
			a.Tags = []string{}
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

// GetActivitiesByIDs loads multiple activities and verifies ownership via user_id.
func (db *DB) GetActivitiesByIDs(ctx context.Context, userID string, ids []string) ([]model.Activity, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT id, user_id, sport_type, title, started_at,
		        elapsed_time_s, moving_time_s, distance_m, elevation_gain_m, parent_id
		 FROM activities
		 WHERE id = ANY($1::uuid[]) AND user_id = $2`,
		ids, userID)
	if err != nil {
		return nil, fmt.Errorf("get activities by ids: %w", err)
	}
	defer rows.Close()
	var result []model.Activity
	for rows.Next() {
		var a model.Activity
		if err := rows.Scan(
			&a.ID, &a.UserID, &a.SportType, &a.Title, &a.StartedAt,
			&a.ElapsedTimeS, &a.MovingTimeS, &a.DistanceM, &a.ElevationGainM, &a.ParentID,
		); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// SetActivitiesParent sets parent_id on multiple activities (with ownership check).
func (db *DB) SetActivitiesParent(ctx context.Context, userID string, activityIDs []string, parentID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE activities SET parent_id = $1
		 WHERE id = ANY($2::uuid[]) AND user_id = $3`,
		parentID, activityIDs, userID)
	return err
}

// AggregateParentMetrics updates a parent activity's metrics by summing its children's values.
func (db *DB) AggregateParentMetrics(ctx context.Context, parentID string) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE activities SET
			elapsed_time_s  = (SELECT SUM(elapsed_time_s)   FROM activities WHERE parent_id = $1),
			moving_time_s   = (SELECT SUM(moving_time_s)    FROM activities WHERE parent_id = $1),
			distance_m      = (SELECT SUM(distance_m)       FROM activities WHERE parent_id = $1),
			elevation_gain_m = (SELECT SUM(elevation_gain_m) FROM activities WHERE parent_id = $1),
			elevation_loss_m = (SELECT SUM(elevation_loss_m) FROM activities WHERE parent_id = $1),
			total_calories  = (SELECT SUM(total_calories)   FROM activities WHERE parent_id = $1)
		WHERE id = $1`, parentID)
	return err
}
