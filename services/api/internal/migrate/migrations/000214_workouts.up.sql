CREATE TABLE workouts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT,
    description     TEXT,
    sport           SMALLINT,
    sub_sport       SMALLINT,
    capabilities    BIGINT,
    num_valid_steps SMALLINT,
    pool_length_m   REAL,
    pool_length_unit TEXT,
    source_hash     TEXT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, source_hash)
);

CREATE TABLE workout_steps (
    id                             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workout_id                     UUID NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    message_index                  SMALLINT NOT NULL,
    step_name                      TEXT,
    notes                          TEXT,
    duration_type                  SMALLINT,
    duration_value                 INTEGER,
    target_type                    SMALLINT,
    target_value                   INTEGER,
    custom_target_low              INTEGER,
    custom_target_high             INTEGER,
    intensity                      SMALLINT,
    exercise_category              SMALLINT,
    exercise_name                  SMALLINT,
    exercise_weight_kg             REAL,
    weight_display_unit            SMALLINT,
    equipment                      SMALLINT,
    secondary_target_type          SMALLINT,
    secondary_target_value         INTEGER,
    secondary_custom_target_low    INTEGER,
    secondary_custom_target_high   INTEGER,
    UNIQUE (workout_id, message_index)
);
CREATE INDEX idx_workout_steps_workout ON workout_steps(workout_id);
