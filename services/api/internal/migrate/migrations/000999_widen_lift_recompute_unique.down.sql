-- Revert idx_bg_jobs_one_running back to the 000285 shape:
-- UNIQUE (user_id, job_type) WHERE status='running'.
DROP INDEX IF EXISTS idx_bg_jobs_one_running;
CREATE UNIQUE INDEX idx_bg_jobs_one_running
    ON background_jobs (user_id, job_type)
    WHERE status = 'running';
