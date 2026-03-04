CREATE TABLE health_weight_entries (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    measured_at_utc TIMESTAMPTZ NOT NULL,
    weight_g INTEGER NOT NULL,
    bmi DOUBLE PRECISION,
    body_fat_pct DOUBLE PRECISION,
    muscle_mass_g INTEGER,
    bone_mass_g INTEGER,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, measured_at_utc)
);
