CREATE TABLE IF NOT EXISTS activity_intervals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    segment_num INT NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    duration_s INT NOT NULL,
    avg_power_w DOUBLE PRECISION,
    avg_heart_rate INT,
    avg_speed_ms DOUBLE PRECISION,
    avg_cadence DOUBLE PRECISION,
    intensity TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_activity_intervals_activity ON activity_intervals(activity_id);
