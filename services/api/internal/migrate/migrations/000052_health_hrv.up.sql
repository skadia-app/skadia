CREATE TABLE health_hrv_snapshots (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    measured_at_utc TIMESTAMPTZ NOT NULL,
    rmssd_ms DOUBLE PRECISION,
    sdrr_ms DOUBLE PRECISION,
    time_of_day TEXT,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, measured_at_utc)
);
SELECT create_hypertable('health_hrv_snapshots', 'measured_at_utc', if_not_exists => TRUE);
