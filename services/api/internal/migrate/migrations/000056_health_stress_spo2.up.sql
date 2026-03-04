CREATE TABLE health_stress_snapshots (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    measured_at_utc TIMESTAMPTZ NOT NULL,
    stress_level DOUBLE PRECISION,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, measured_at_utc)
);

CREATE TABLE health_spo2_snapshots (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    measured_at_utc TIMESTAMPTZ NOT NULL,
    spo2_pct DOUBLE PRECISION,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, measured_at_utc)
);
