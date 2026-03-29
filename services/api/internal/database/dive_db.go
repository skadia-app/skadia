package database

import (
	"context"

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
