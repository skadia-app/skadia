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
GROUP BY user_id, time_bucket('1 day', ts);

SELECT add_continuous_aggregate_policy('health_monitoring_daily',
    start_offset    => INTERVAL '30 days',
    end_offset      => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour');
