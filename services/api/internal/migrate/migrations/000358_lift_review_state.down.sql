-- Phase 4d + Phase 1 (detection-mode toggles) + Phase 6 (F3 widened bg-jobs index) rollback.
-- Drop in dependency order: index first, then columns.
-- Run grep first: grep -r "rejected_by_user\|lifts_acknowledged_at\|lift_detection_osm_override\|lift_detection_heuristic_override\|enable_lift_detection_osm\|enable_lift_detection_heuristic" services/api/ frontend/src/
-- must be empty (global CLAUDE.md "Database column drops" rule).

-- Phase 6 (F3) revert background_jobs uniqueness to the 000285 shape:
-- UNIQUE (user_id, job_type) WHERE status='running'.
DROP INDEX IF EXISTS idx_bg_jobs_one_lift_recompute;
DROP INDEX IF EXISTS idx_bg_jobs_one_running;
CREATE UNIQUE INDEX idx_bg_jobs_one_running
    ON background_jobs (user_id, job_type)
    WHERE status = 'running';

-- Phase 1 toggle columns (drop before parent columns).
ALTER TABLE activities DROP COLUMN IF EXISTS lift_detection_heuristic_override;
ALTER TABLE activities DROP COLUMN IF EXISTS lift_detection_osm_override;
ALTER TABLE user_enrichment_settings DROP COLUMN IF EXISTS enable_lift_detection_heuristic;
ALTER TABLE user_enrichment_settings DROP COLUMN IF EXISTS enable_lift_detection_osm;

-- Phase 4d columns.
DROP INDEX IF EXISTS idx_lift_candidates_active;
ALTER TABLE activity_lift_candidates DROP COLUMN IF EXISTS rejected_by_user;
ALTER TABLE activities DROP COLUMN IF EXISTS lifts_acknowledged_at;
