CREATE TABLE health_blood_pressure (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    measured_at_utc TIMESTAMPTZ NOT NULL,
    systolic INTEGER,
    diastolic INTEGER,
    pulse INTEGER,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, measured_at_utc)
);

CREATE TABLE health_fitness_age (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    calendar_date DATE NOT NULL,
    fitness_age DOUBLE PRECISION,
    source_type TEXT NOT NULL DEFAULT 'manual',
    extra_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, calendar_date)
);
