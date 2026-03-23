CREATE TABLE activity_sets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id         UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    message_index       SMALLINT NOT NULL,
    timestamp           TIMESTAMPTZ,
    start_time          TIMESTAMPTZ,
    duration_s          REAL,
    set_type            SMALLINT,
    repetitions         INTEGER,
    weight_kg           REAL,
    weight_display_unit SMALLINT,
    exercise_categories SMALLINT[],
    exercise_subtypes   SMALLINT[],
    wkt_step_index      SMALLINT,
    UNIQUE (activity_id, message_index)
);
CREATE INDEX idx_activity_sets_activity ON activity_sets(activity_id);
