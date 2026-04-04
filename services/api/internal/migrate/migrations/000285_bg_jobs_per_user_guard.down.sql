-- Revert to the global per-job-type uniqueness constraint.
DROP INDEX IF EXISTS idx_bg_jobs_one_running;
CREATE UNIQUE INDEX idx_bg_jobs_one_running
    ON background_jobs (job_type)
    WHERE status = 'running';
