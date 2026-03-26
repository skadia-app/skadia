-- Enforce at most one running job per job_type.
-- This partial unique index makes InsertBackgroundJobGuarded race-free under
-- READ COMMITTED: the second concurrent INSERT will fail with a unique_violation
-- (code 23505) rather than silently inserting a duplicate running job.
CREATE UNIQUE INDEX idx_bg_jobs_one_running
    ON background_jobs (job_type)
    WHERE status = 'running';
