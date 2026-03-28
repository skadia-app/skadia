package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// UpsertTrainingZones upserts a training zone for a given sport/sub_sport/zone_type.
// Zone values use EXCLUDED (NOT COALESCE) — latest import wins since zones are user-defined settings.
func (db *DB) UpsertTrainingZones(ctx context.Context, zone *model.TrainingZone) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO training_zones (
			user_id, sport, sub_sport, zone_type,
			max_heart_rate, threshold_heart_rate, functional_threshold_power,
			hr_calc_type, pwr_calc_type,
			zone_names, zone_highs, source_type, threshold_speed_mps
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (user_id, sport, sub_sport, zone_type) DO UPDATE SET
			max_heart_rate             = EXCLUDED.max_heart_rate,
			threshold_heart_rate       = EXCLUDED.threshold_heart_rate,
			functional_threshold_power = EXCLUDED.functional_threshold_power,
			hr_calc_type               = EXCLUDED.hr_calc_type,
			pwr_calc_type              = EXCLUDED.pwr_calc_type,
			zone_names                 = EXCLUDED.zone_names,
			zone_highs                 = EXCLUDED.zone_highs,
			threshold_speed_mps        = EXCLUDED.threshold_speed_mps,
			source_type = CASE
				WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(training_zones.source_type)
				THEN EXCLUDED.source_type
				ELSE training_zones.source_type
			END,
			updated_at = NOW()
	`, zone.UserID, zone.Sport, zone.SubSport, zone.ZoneType,
		zone.MaxHeartRate, zone.ThresholdHeartRate, zone.FunctionalThresholdPower,
		zone.HrCalcType, zone.PwrCalcType,
		zone.ZoneNames, zone.ZoneHighs, zone.SourceType, zone.ThresholdSpeedMps)
	return err
}

// GetTrainingZones returns all training zones for a user, optionally filtered by sport.
func (db *DB) GetTrainingZones(ctx context.Context, userID string, sport, subSport *int16) ([]model.TrainingZone, error) {
	query := `
		SELECT id, user_id, sport, sub_sport, zone_type,
		       max_heart_rate, threshold_heart_rate, functional_threshold_power,
		       threshold_speed_mps,
		       hr_calc_type, pwr_calc_type,
		       zone_names, zone_highs, source_type, created_at, updated_at
		FROM training_zones
		WHERE user_id = $1`
	args := []any{userID}

	if sport != nil {
		query += ` AND sport = $2`
		args = append(args, *sport)
		if subSport != nil {
			query += ` AND sub_sport = $3`
			args = append(args, *subSport)
		}
	}
	query += ` ORDER BY sport, sub_sport, zone_type`

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query training zones: %w", err)
	}
	defer rows.Close()

	var zones []model.TrainingZone
	for rows.Next() {
		var z model.TrainingZone
		if err := rows.Scan(
			&z.ID, &z.UserID, &z.Sport, &z.SubSport, &z.ZoneType,
			&z.MaxHeartRate, &z.ThresholdHeartRate, &z.FunctionalThresholdPower,
			&z.ThresholdSpeedMps,
			&z.HrCalcType, &z.PwrCalcType,
			&z.ZoneNames, &z.ZoneHighs, &z.SourceType, &z.CreatedAt, &z.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan training zone: %w", err)
		}
		zones = append(zones, z)
	}
	return zones, rows.Err()
}
