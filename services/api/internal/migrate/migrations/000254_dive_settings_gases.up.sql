CREATE TABLE IF NOT EXISTS activity_dive_settings (
    activity_id UUID PRIMARY KEY REFERENCES activities(id) ON DELETE CASCADE,
    gf_low SMALLINT,
    gf_high SMALLINT,
    water_type TEXT,
    water_density REAL,
    deco_model TEXT,
    po2_warn REAL,
    po2_critical REAL,
    po2_deco REAL,
    safety_stop_enabled BOOLEAN,
    safety_stop_time_s SMALLINT,
    repeat_dive_interval_s SMALLINT,
    ccr_low_setpoint REAL,
    ccr_low_setpoint_depth_m REAL,
    ccr_high_setpoint REAL,
    ccr_high_setpoint_depth_m REAL
);

CREATE TABLE IF NOT EXISTS activity_dive_gases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    gas_index SMALLINT NOT NULL,
    helium_pct SMALLINT,
    oxygen_pct SMALLINT,
    status TEXT,
    mode TEXT,
    UNIQUE (activity_id, gas_index)
);
