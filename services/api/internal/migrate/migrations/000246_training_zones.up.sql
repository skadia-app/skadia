CREATE TABLE training_zones (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sport       SMALLINT NOT NULL,
    sub_sport   SMALLINT NOT NULL DEFAULT 0,
    zone_type   TEXT NOT NULL,
    max_heart_rate             SMALLINT,
    threshold_heart_rate       SMALLINT,
    functional_threshold_power INTEGER,
    hr_calc_type               SMALLINT,
    pwr_calc_type              SMALLINT,
    zone_names   TEXT[],
    zone_highs   REAL[],
    source_type  TEXT NOT NULL DEFAULT 'manual',
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, sport, sub_sport, zone_type)
);
