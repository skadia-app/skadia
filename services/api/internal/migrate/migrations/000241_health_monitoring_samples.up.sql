CREATE TABLE health_monitoring_samples (
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ts                   TIMESTAMPTZ NOT NULL,
    heart_rate           SMALLINT,
    activity_type        SMALLINT,
    intensity            SMALLINT,
    temperature_c        REAL,
    temperature_min_c    REAL,
    temperature_max_c    REAL,
    stress               SMALLINT CHECK (stress IS NULL OR stress BETWEEN 0 AND 100),
    body_battery         SMALLINT,
    body_battery_delta   SMALLINT,
    respiration_rate     REAL,
    sleep_level          SMALLINT,
    spo2_pct             SMALLINT,
    spo2_confidence      SMALLINT,
    wrist_temperature_c  REAL,
    PRIMARY KEY (user_id, ts)
);

SELECT create_hypertable('health_monitoring_samples', 'ts',
    chunk_time_interval => INTERVAL '1 day');

ALTER TABLE health_monitoring_samples SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'user_id',
    timescaledb.compress_orderby = 'ts DESC'
);

SELECT add_compression_policy('health_monitoring_samples', INTERVAL '7 days');
