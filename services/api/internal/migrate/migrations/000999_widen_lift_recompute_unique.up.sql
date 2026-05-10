-- Widen the per-user "one running job per type" guard to also include
-- entity_id, so that lift_recompute jobs for two different activities of
-- the same user can run concurrently while still rejecting a duplicate
-- run on the same activity.
--
-- Old shape (000285): UNIQUE (user_id, job_type) WHERE status='running'
--   - blocked any second lift_recompute regardless of activity_id
--
-- New shape: UNIQUE (user_id, job_type, COALESCE(entity_id, ''::text))
--            WHERE status IN ('running','queued')
--   - per-(user, job_type, entity_id) uniqueness so different activities
--     can recompute in parallel
--   - COALESCE handles non-entity-scoped jobs (entity_id IS NULL): they
--     all collapse to the empty-string key, preserving the original
--     "one running per (user, job_type)" semantics for those callers
--   - widened to also cover 'queued' — this prevents a fast-follow request
--     from sneaking in between dispatch and worker pickup
DROP INDEX IF EXISTS idx_bg_jobs_one_running;
CREATE UNIQUE INDEX idx_bg_jobs_one_running
    ON background_jobs (user_id, job_type, (COALESCE(entity_id, '')))
    WHERE status IN ('running', 'queued');
