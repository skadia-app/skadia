package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// InsertDiveSettings inserts a DiveSettings record for the given activity.
// On conflict (re-import) the existing row is left unchanged.
func (db *DB) InsertDiveSettings(ctx context.Context, ds model.DiveSettings) error {
	_, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_dive_settings (
			activity_id, gf_low, gf_high, water_type, water_density,
			deco_model, po2_warn, po2_critical, po2_deco,
			safety_stop_enabled, safety_stop_time_s, repeat_dive_interval_s,
			ccr_low_setpoint, ccr_low_setpoint_depth_m,
			ccr_high_setpoint, ccr_high_setpoint_depth_m
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (activity_id) DO NOTHING`,
		ds.ActivityID, ds.GfLow, ds.GfHigh, ds.WaterType, ds.WaterDensity,
		ds.DecoModel, ds.Po2Warn, ds.Po2Critical, ds.Po2Deco,
		ds.SafetyStopEnabled, ds.SafetyStopTimeS, ds.RepeatDiveIntervalS,
		ds.CcrLowSetpoint, ds.CcrLowSetpointDepthM,
		ds.CcrHighSetpoint, ds.CcrHighSetpointDepthM,
	)
	return err
}

// GetDiveSettings returns the dive settings for the given activity, or nil if none.
func (db *DB) GetDiveSettings(ctx context.Context, activityID string) (*model.DiveSettings, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT activity_id, gf_low, gf_high, water_type, water_density,
			deco_model, po2_warn, po2_critical, po2_deco,
			safety_stop_enabled, safety_stop_time_s, repeat_dive_interval_s,
			ccr_low_setpoint, ccr_low_setpoint_depth_m,
			ccr_high_setpoint, ccr_high_setpoint_depth_m
		FROM activity_dive_settings
		WHERE activity_id = $1`, activityID)

	var ds model.DiveSettings
	err := row.Scan(
		&ds.ActivityID, &ds.GfLow, &ds.GfHigh, &ds.WaterType, &ds.WaterDensity,
		&ds.DecoModel, &ds.Po2Warn, &ds.Po2Critical, &ds.Po2Deco,
		&ds.SafetyStopEnabled, &ds.SafetyStopTimeS, &ds.RepeatDiveIntervalS,
		&ds.CcrLowSetpoint, &ds.CcrLowSetpointDepthM,
		&ds.CcrHighSetpoint, &ds.CcrHighSetpointDepthM,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get dive settings: %w", err)
	}
	return &ds, nil
}

// InsertDiveGases inserts all DiveGas records for the given activity.
// On conflict (re-import, same gas_index) the existing row is left unchanged.
func (db *DB) InsertDiveGases(ctx context.Context, gases []model.DiveGas) error {
	for _, g := range gases {
		_, err := db.Pool.Exec(ctx,
			`INSERT INTO activity_dive_gases (
				activity_id, gas_index, helium_pct, oxygen_pct, status, mode
			) VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (activity_id, gas_index) DO NOTHING`,
			g.ActivityID, g.GasIndex, g.HeliumPct, g.OxygenPct, g.Status, g.Mode,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetDivePo2Stats returns min/max pO₂ and the depth at those exact points.
// Uses window functions to correlate pO₂ extremes with their depth reading.
func (db *DB) GetDivePo2Stats(ctx context.Context, activityID string) (*model.DivePo2Stats, error) {
	const q = `
        WITH ranked AS (
            SELECT
                po2,
                depth_m,
                ROW_NUMBER() OVER (ORDER BY po2 ASC)  AS rn_min,
                ROW_NUMBER() OVER (ORDER BY po2 DESC) AS rn_max
            FROM activity_streams
            WHERE activity_id = $1
              AND po2 IS NOT NULL
              AND po2 > 0
        )
        SELECT
            (SELECT po2     FROM ranked WHERE rn_min = 1) AS min_po2,
            (SELECT depth_m FROM ranked WHERE rn_min = 1) AS min_po2_depth,
            (SELECT po2     FROM ranked WHERE rn_max = 1) AS max_po2,
            (SELECT depth_m FROM ranked WHERE rn_max = 1) AS max_po2_depth
    `
	var s model.DivePo2Stats
	var minDepth, maxDepth *float64
	err := db.Pool.QueryRow(ctx, q, activityID).Scan(
		&s.MinPo2, &minDepth, &s.MaxPo2, &maxDepth,
	)
	if err != nil {
		return nil, err
	}
	if s.MinPo2 == 0 && s.MaxPo2 == 0 {
		return nil, nil // no pO₂ data
	}
	s.MinPo2Depth = minDepth
	s.MaxPo2Depth = maxDepth
	return &s, nil
}

// GetDiveGases returns all gas mixes for the given activity.
func (db *DB) GetDiveGases(ctx context.Context, activityID string) ([]model.DiveGas, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT activity_id, gas_index, helium_pct, oxygen_pct, status, mode
		FROM activity_dive_gases
		WHERE activity_id = $1
		ORDER BY gas_index`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get dive gases: %w", err)
	}
	defer rows.Close()

	var gases []model.DiveGas
	for rows.Next() {
		var g model.DiveGas
		if err := rows.Scan(&g.ActivityID, &g.GasIndex, &g.HeliumPct, &g.OxygenPct, &g.Status, &g.Mode); err != nil {
			return nil, err
		}
		gases = append(gases, g)
	}
	return gases, rows.Err()
}
