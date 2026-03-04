-- health_hrv_night_values: 5-minute RMSSD measurements during sleep
-- Extracted from Garmin monitoring FIT files (file type 205, message 281/hrv_value).
-- Each row represents one 5-minute window during a night's sleep.
CREATE TABLE IF NOT EXISTS health_hrv_night_values (
    user_id      UUID          NOT NULL REFERENCES users(id),
    ts           TIMESTAMPTZ   NOT NULL,
    rmssd_ms     DOUBLE PRECISION NOT NULL,
    source_type  TEXT          NOT NULL DEFAULT 'garmin_fit',
    PRIMARY KEY (user_id, ts)
);
SELECT create_hypertable('health_hrv_night_values', 'ts', if_not_exists => TRUE);
