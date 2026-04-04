CREATE TABLE best_efforts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    sport_group TEXT NOT NULL,
    elapsed_time_s INTEGER,
    value_numeric DOUBLE PRECISION,
    achieved_at TIMESTAMPTZ NOT NULL,
    stream_start_idx INTEGER,
    stream_end_idx INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (activity_id, category, sport_group)
);

CREATE INDEX idx_best_efforts_ranking
    ON best_efforts (user_id, category, sport_group, elapsed_time_s ASC NULLS LAST);
CREATE INDEX idx_best_efforts_power
    ON best_efforts (user_id, category, sport_group, value_numeric DESC NULLS LAST);
CREATE INDEX idx_best_efforts_activity
    ON best_efforts (activity_id);

CREATE TABLE best_effort_exclusions (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    category TEXT NOT NULL DEFAULT '__all__',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, activity_id, category)
);
