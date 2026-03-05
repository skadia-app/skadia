CREATE TABLE IF NOT EXISTS activity_lift_candidates (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    activity_id  UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    started_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ NOT NULL,
    duration_s   INTEGER NOT NULL,
    ascent_m     DOUBLE PRECISION NOT NULL,
    confidence   TEXT NOT NULL DEFAULT 'medium',
    confirmed    BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (activity_id, started_at)
);
CREATE INDEX ON activity_lift_candidates (activity_id);
