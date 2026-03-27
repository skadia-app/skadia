package garmin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureMonitoringDailyCA creates the health_monitoring_daily Continuous Aggregate
// if it does not exist yet. This runs outside golang-migrate because TimescaleDB
// CAs cannot be created inside a transaction (golang-migrate wraps each migration
// in one). See migration 000242 (no-op placeholder).
func EnsureMonitoringDailyCA(ctx context.Context, pool *pgxpool.Pool) error {
	// Check if the CA already exists.
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM timescaledb_information.continuous_aggregates WHERE view_name = 'health_monitoring_daily')`,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check CA existence: %w", err)
	}
	if exists {
		slog.Debug("health_monitoring_daily CA already exists")
		return nil
	}

	// Check that the underlying hypertable exists (migration 000241).
	var htExists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'health_monitoring_samples')`,
	).Scan(&htExists)
	if err != nil || !htExists {
		slog.Info("health_monitoring_samples hypertable not found, skipping CA creation")
		return nil
	}

	slog.Info("creating health_monitoring_daily Continuous Aggregate")

	_, err = pool.Exec(ctx, `
		CREATE MATERIALIZED VIEW health_monitoring_daily
		WITH (timescaledb.continuous) AS
		SELECT
			user_id,
			time_bucket('1 day', ts) AS day,
			avg(stress)::REAL              AS stress_avg,
			min(stress)                    AS stress_min,
			max(stress)                    AS stress_max,
			min(body_battery)              AS body_battery_min,
			max(body_battery)              AS body_battery_max,
			avg(heart_rate)::REAL          AS heart_rate_avg,
			min(heart_rate)                AS heart_rate_min,
			max(heart_rate)                AS heart_rate_max,
			avg(respiration_rate)::REAL    AS respiration_rate_avg,
			avg(spo2_pct)::REAL            AS spo2_avg
		FROM health_monitoring_samples
		WHERE stress IS NOT NULL
		   OR body_battery IS NOT NULL
		   OR heart_rate IS NOT NULL
		   OR respiration_rate IS NOT NULL
		   OR spo2_pct IS NOT NULL
		   OR sleep_level IS NOT NULL
		GROUP BY user_id, time_bucket('1 day', ts)
		WITH NO DATA
	`)
	if err != nil {
		return fmt.Errorf("create CA: %w", err)
	}

	_, err = pool.Exec(ctx, `
		SELECT add_continuous_aggregate_policy('health_monitoring_daily',
			start_offset    => INTERVAL '30 days',
			end_offset      => INTERVAL '1 hour',
			schedule_interval => INTERVAL '1 hour',
			if_not_exists    => true)
	`)
	if err != nil {
		return fmt.Errorf("add CA policy: %w", err)
	}

	slog.Info("health_monitoring_daily CA created successfully")
	return nil
}
