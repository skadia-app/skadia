CREATE TABLE ml_system_settings (
    setting_key TEXT PRIMARY KEY,
    value_json  JSONB NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE ml_user_settings (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    setting_key TEXT NOT NULL,
    value_json  JSONB NOT NULL,
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, setting_key)
);

-- Default nightly job config
INSERT INTO ml_system_settings (setting_key, value_json) VALUES
    ('nightly_feature_engineering', '{"enabled": true, "time": "02:00", "description": "NeuroKit2 HRV + TSFEL Features"}'),
    ('nightly_tsfm_analysis', '{"enabled": false, "time": "03:00", "description": "TSPulse Anomaly + TTM Forecasts"}'),
    ('nightly_changepoint_detection', '{"enabled": true, "time": "04:00", "description": "ruptures PELT Changepoints"}');
