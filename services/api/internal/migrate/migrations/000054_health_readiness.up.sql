CREATE TABLE health_readiness_scores (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    calendar_date DATE NOT NULL,
    score INTEGER,
    level TEXT,
    sleep_score INTEGER,
    hrv_weekly_avg_ms DOUBLE PRECISION,
    recovery_time_min INTEGER,
    acwr_pct DOUBLE PRECISION,
    hrv_factor_pct DOUBLE PRECISION,
    sleep_history_factor_pct DOUBLE PRECISION,
    stress_history_factor_pct DOUBLE PRECISION,
    recovery_time_factor_pct DOUBLE PRECISION,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, calendar_date)
);
