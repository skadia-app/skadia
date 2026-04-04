-- Replace the global per-job-type uniqueness constraint with a per-user constraint.
-- Previously idx_bg_jobs_one_running enforced at most one running job globally
-- per job_type, which blocked a user from starting a job that another user was
-- already running. The new index scopes the constraint to (user_id, job_type)
-- so different users can run the same job type concurrently.
DROP INDEX IF EXISTS idx_bg_jobs_one_running;
CREATE UNIQUE INDEX idx_bg_jobs_one_running
    ON background_jobs (user_id, job_type)
    WHERE status = 'running';
