CREATE TABLE background_jobs (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT        NOT NULL,
    job_type    TEXT        NOT NULL,
    entity_id   TEXT,
    entity_name TEXT,
    status      TEXT        NOT NULL DEFAULT 'running',
    progress_current  INT   NOT NULL DEFAULT 0,
    progress_total    INT   NOT NULL DEFAULT 0,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX background_jobs_user_status ON background_jobs (user_id, status);
CREATE INDEX background_jobs_finished    ON background_jobs (finished_at)
    WHERE finished_at IS NOT NULL;
