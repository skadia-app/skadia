package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// upsertZonesConflictClause is the shared ON CONFLICT SET clause used by
// both the priority-checked and manual-override upsert paths.
const upsertZonesConflictClause = `
			max_heart_rate             = EXCLUDED.max_heart_rate,
			threshold_heart_rate       = EXCLUDED.threshold_heart_rate,
			functional_threshold_power = EXCLUDED.functional_threshold_power,
			hr_calc_type               = EXCLUDED.hr_calc_type,
			pwr_calc_type              = EXCLUDED.pwr_calc_type,
			zone_names                 = EXCLUDED.zone_names,
			zone_highs                 = EXCLUDED.zone_highs,
			threshold_speed_mps        = EXCLUDED.threshold_speed_mps,
			updated_at                 = NOW()`

// upsertTrainingZonesWithPriority inserts or updates a training zone row,
// letting source_type_priority() decide which source_type wins on conflict.
// The INSERT uses zone.SourceType as-is ($12) alongside the other zone fields ($13).
func (db *DB) upsertTrainingZonesWithPriority(ctx context.Context, zone *model.TrainingZone) error {
	sql := `
		INSERT INTO training_zones (
			user_id, sport, sub_sport, zone_type,
			max_heart_rate, threshold_heart_rate, functional_threshold_power,
			hr_calc_type, pwr_calc_type,
			zone_names, zone_highs, source_type, threshold_speed_mps
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (user_id, sport, sub_sport, zone_type) DO UPDATE SET
			` + upsertZonesConflictClause + `,
			source_type = CASE
				WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(training_zones.source_type)
				THEN EXCLUDED.source_type
				ELSE training_zones.source_type
			END`
	_, err := db.Pool.Exec(ctx, sql,
		zone.UserID, zone.Sport, zone.SubSport, zone.ZoneType,
		zone.MaxHeartRate, zone.ThresholdHeartRate, zone.FunctionalThresholdPower,
		zone.HrCalcType, zone.PwrCalcType,
		zone.ZoneNames, zone.ZoneHighs, zone.SourceType, zone.ThresholdSpeedMps)
	return err
}

// upsertTrainingZonesForceOverride inserts or updates a training zone row,
// unconditionally setting source_type = 'manual_override' on both INSERT and
// conflict UPDATE — bypassing source_type_priority() entirely.
func (db *DB) upsertTrainingZonesForceOverride(ctx context.Context, zone *model.TrainingZone) error {
	sql := `
		INSERT INTO training_zones (
			user_id, sport, sub_sport, zone_type,
			max_heart_rate, threshold_heart_rate, functional_threshold_power,
			hr_calc_type, pwr_calc_type,
			zone_names, zone_highs, source_type, threshold_speed_mps
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'manual_override', $12)
		ON CONFLICT (user_id, sport, sub_sport, zone_type) DO UPDATE SET
			` + upsertZonesConflictClause + `,
			source_type = 'manual_override'`
	_, err := db.Pool.Exec(ctx, sql,
		zone.UserID, zone.Sport, zone.SubSport, zone.ZoneType,
		zone.MaxHeartRate, zone.ThresholdHeartRate, zone.FunctionalThresholdPower,
		zone.HrCalcType, zone.PwrCalcType,
		zone.ZoneNames, zone.ZoneHighs, zone.ThresholdSpeedMps)
	return err
}

// UpsertTrainingZones upserts a training zone for a given sport/sub_sport/zone_type.
// Zone values use EXCLUDED (NOT COALESCE) — latest import wins since zones are user-defined settings.
// source_type_priority() decides which source_type wins on conflict.
func (db *DB) UpsertTrainingZones(ctx context.Context, zone *model.TrainingZone) error {
	return db.upsertTrainingZonesWithPriority(ctx, zone)
}

// UpsertTrainingZonesManual upserts a training zone with source_type = 'manual_override'.
// Unlike UpsertTrainingZones, this bypasses source_type_priority() so that a manual
// user edit always wins over any auto-detected value (priority 60, the highest).
func (db *DB) UpsertTrainingZonesManual(ctx context.Context, zone *model.TrainingZone) error {
	return db.upsertTrainingZonesForceOverride(ctx, zone)
}

// GetTrainingZones returns all training zones for a user, optionally filtered by sport
// and sub_sport.  Pass nil for sport to return zones for all sports.
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

// GetAllTrainingZoneSports returns the union of:
//   - sports that already have stored zone rows for the user
//   - distinct fit_sport_id values present in the user's activities
//
// This gives the complete list of sports for which the zone editor should
// offer configuration UI, even when no zones have been saved yet.
func (db *DB) GetAllTrainingZoneSports(ctx context.Context, userID string) ([]int16, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT DISTINCT sport AS sport_id FROM training_zones WHERE user_id = $1
		UNION
		SELECT DISTINCT fit_sport_id AS sport_id FROM activities WHERE user_id = $1 AND fit_sport_id IS NOT NULL
		ORDER BY sport_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query training zone sports: %w", err)
	}
	defer rows.Close()

	var ids []int16
	for rows.Next() {
		var id int16
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan training zone sport: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
