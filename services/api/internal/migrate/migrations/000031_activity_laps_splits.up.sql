-- Activity Laps: Universal lap data (auto-lap, manual laps, swim groups)
CREATE TABLE activity_laps (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id           UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    message_index         INTEGER NOT NULL,
    start_time            TIMESTAMPTZ,
    end_time              TIMESTAMPTZ,
    lap_trigger           TEXT,
    sport                 TEXT,
    sub_sport             TEXT,
    total_elapsed_time_s  DOUBLE PRECISION,
    total_timer_time_s    DOUBLE PRECISION,
    total_moving_time_s   DOUBLE PRECISION,
    total_distance_m      DOUBLE PRECISION,
    total_ascent_m        INTEGER,
    total_descent_m       INTEGER,
    total_calories        INTEGER,
    avg_speed_ms          DOUBLE PRECISION,
    max_speed_ms          DOUBLE PRECISION,
    avg_heart_rate        INTEGER,
    max_heart_rate        INTEGER,
    min_heart_rate        INTEGER,
    avg_cadence           INTEGER,
    max_cadence           INTEGER,
    avg_power_w           INTEGER,
    max_power_w           INTEGER,
    normalized_power_w    INTEGER,
    avg_temperature_c     INTEGER,
    start_lat             DOUBLE PRECISION,
    start_lng             DOUBLE PRECISION,
    end_lat               DOUBLE PRECISION,
    end_lng               DOUBLE PRECISION,
    total_work_j          INTEGER,
    intensity             TEXT,
    swim_stroke           TEXT,
    num_lengths           INTEGER,
    num_active_lengths    INTEGER,
    avg_altitude_m        DOUBLE PRECISION,
    max_altitude_m        DOUBLE PRECISION,
    min_altitude_m        DOUBLE PRECISION,
    UNIQUE (activity_id, message_index)
);
CREATE INDEX idx_activity_laps_activity_id ON activity_laps (activity_id);

-- Activity Splits: Split data for climbing, skiing, intervals
CREATE TABLE activity_splits (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id           UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    message_index         INTEGER NOT NULL,
    split_type            TEXT,
    start_time            TIMESTAMPTZ,
    end_time              TIMESTAMPTZ,
    total_elapsed_time_s  DOUBLE PRECISION,
    total_timer_time_s    DOUBLE PRECISION,
    total_moving_time_s   DOUBLE PRECISION,
    total_distance_m      DOUBLE PRECISION,
    total_ascent_m        INTEGER,
    total_descent_m       INTEGER,
    total_calories        INTEGER,
    avg_speed_ms          DOUBLE PRECISION,
    max_speed_ms          DOUBLE PRECISION,
    avg_vert_speed_ms     DOUBLE PRECISION,
    start_lat             DOUBLE PRECISION,
    start_lng             DOUBLE PRECISION,
    end_lat               DOUBLE PRECISION,
    end_lng               DOUBLE PRECISION,
    start_elevation_m     DOUBLE PRECISION,
    UNIQUE (activity_id, message_index)
);
CREATE INDEX idx_activity_splits_activity_id ON activity_splits (activity_id);

-- Activity Split Summaries: Aggregates per split type
CREATE TABLE activity_split_summaries (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id           UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    message_index         INTEGER NOT NULL,
    split_type            TEXT,
    num_splits            INTEGER,
    total_timer_time_s    DOUBLE PRECISION,
    total_moving_time_s   DOUBLE PRECISION,
    total_distance_m      DOUBLE PRECISION,
    total_ascent_m        INTEGER,
    total_descent_m       INTEGER,
    total_calories        INTEGER,
    avg_speed_ms          DOUBLE PRECISION,
    max_speed_ms          DOUBLE PRECISION,
    avg_vert_speed_ms     DOUBLE PRECISION,
    avg_heart_rate        INTEGER,
    max_heart_rate        INTEGER,
    UNIQUE (activity_id, message_index)
);
CREATE INDEX idx_activity_split_summaries_activity_id ON activity_split_summaries (activity_id);

-- Activity Lengths: Swim pool lengths (pool traversals)
CREATE TABLE activity_lengths (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id           UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    message_index         INTEGER NOT NULL,
    start_time            TIMESTAMPTZ,
    end_time              TIMESTAMPTZ,
    total_elapsed_time_s  DOUBLE PRECISION,
    total_timer_time_s    DOUBLE PRECISION,
    total_strokes         INTEGER,
    total_calories        INTEGER,
    avg_speed_ms          DOUBLE PRECISION,
    avg_swimming_cadence  INTEGER,
    swim_stroke           TEXT,
    length_type           TEXT,
    UNIQUE (activity_id, message_index)
);
CREATE INDEX idx_activity_lengths_activity_id ON activity_lengths (activity_id);

-- Activity ClimbPro Events
CREATE TABLE activity_climb_pro_events (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id           UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    timestamp             TIMESTAMPTZ,
    event_type            TEXT,
    climb_number          INTEGER,
    climb_category        INTEGER,
    current_dist_m        DOUBLE PRECISION,
    lat                   DOUBLE PRECISION,
    lng                   DOUBLE PRECISION
);
CREATE INDEX idx_activity_climb_pro_events_activity_id ON activity_climb_pro_events (activity_id);
