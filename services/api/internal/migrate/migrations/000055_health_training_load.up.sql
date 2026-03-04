CREATE TABLE health_training_load (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    calendar_date DATE NOT NULL,
    atl_acute DOUBLE PRECISION,
    ctl_chronic DOUBLE PRECISION,
    acwr_pct DOUBLE PRECISION,
    acwr_status TEXT,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, calendar_date)
);
