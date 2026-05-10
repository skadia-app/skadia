-- Phase 4d — Soft-Reject-Persistenz + Acknowledgment-State + Detection-Mode Toggles.
--
-- rejected_by_user: soft-reject flag on lift candidates. Hidden from default
-- queries and aggregates. Survives re-enrichment unless explicitly cleared via
-- /lifts/recalculate-all. User-authored data — not regenerable on re-import.
--
-- idx_lift_candidates_active: partial index for the common query path (active
-- = not rejected). Must be dropped before rejected_by_user is dropped.
--
-- lifts_acknowledged_at: set when the user has reviewed lift candidates
-- (confirm-all / reject-all / single-action via review modal). NULL means the
-- modal should show on the next visit.
--
-- ADD COLUMN ... DEFAULT FALSE in Postgres 11+ is a catalog-only operation
-- (no table rewrite) when the column is NOT NULL with a constant default.

ALTER TABLE activity_lift_candidates
    ADD COLUMN IF NOT EXISTS rejected_by_user BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_lift_candidates_active
    ON activity_lift_candidates (activity_id)
    WHERE NOT rejected_by_user;

ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS lifts_acknowledged_at TIMESTAMPTZ;

COMMENT ON COLUMN activity_lift_candidates.rejected_by_user IS
    'Soft-reject flag. Hidden from default queries / aggregates. Survives re-enrichment unless explicitly cleared via /lifts/recalculate-all.';

COMMENT ON COLUMN activities.lifts_acknowledged_at IS
    'Set when the user has reviewed lift candidates (confirm-all/reject-all/single-action via review modal). NULL means modal should show on next visit.';

-- Phase 1 of lift-detection-toggles: per-user sub-toggles and per-activity overrides.
--
-- enable_lift_detection_osm / enable_lift_detection_heuristic:
--   Sub-toggles of enable_lift_detection. Allow users to independently disable
--   OSM-snap stages (1-3) or the heuristic detector without turning off the
--   parent toggle. DEFAULT TRUE keeps behaviour unchanged for existing users.
--
-- lift_detection_osm_override / lift_detection_heuristic_override (activities):
--   Per-activity overrides. NULL means inherit the user-level setting.
--   Set explicitly to override for a single activity without changing user prefs.

ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS enable_lift_detection_osm BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE user_enrichment_settings
    ADD COLUMN IF NOT EXISTS enable_lift_detection_heuristic BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS lift_detection_osm_override BOOLEAN;

ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS lift_detection_heuristic_override BOOLEAN;

COMMENT ON COLUMN user_enrichment_settings.enable_lift_detection_osm IS
    'Sub-toggle of enable_lift_detection. When false, OSM-snap stages (1-3) are skipped for new imports.';
COMMENT ON COLUMN user_enrichment_settings.enable_lift_detection_heuristic IS
    'Sub-toggle of enable_lift_detection. When false, DetectHeuristic is skipped — pure-OSM-mode if osm sub-toggle is true.';
COMMENT ON COLUMN activities.lift_detection_osm_override IS
    'Per-activity override of user_enrichment_settings.enable_lift_detection_osm. NULL = inherit user setting.';
COMMENT ON COLUMN activities.lift_detection_heuristic_override IS
    'Per-activity override of user_enrichment_settings.enable_lift_detection_heuristic. NULL = inherit user setting.';

-- Phase 6 (toggles plan-end critic F3): widen background_jobs running-job guard.
--
-- Old shape (000285): UNIQUE (user_id, job_type) WHERE status='running'
--   - blocked any second lift_recompute regardless of activity_id, so a user
--     re-detecting two activities back-to-back saw the 409 even though the
--     work is independent.
--
-- New shape: UNIQUE (user_id, job_type, COALESCE(entity_id, '')) WHERE status IN ('running','queued')
--   - per-(user, job_type, entity_id) uniqueness so different activities can
--     recompute in parallel.
--   - COALESCE handles non-entity-scoped jobs (entity_id IS NULL): they all
--     collapse to the empty-string key, preserving the original
--     "one running per (user, job_type)" semantics for those callers.
--   - widened to also cover 'queued' so a fast-follow request between
--     dispatch and worker pickup is rejected rather than slipping through.
DROP INDEX IF EXISTS idx_bg_jobs_one_running;
CREATE UNIQUE INDEX idx_bg_jobs_one_running
    ON background_jobs (user_id, job_type, (COALESCE(entity_id, '')))
    WHERE status IN ('running', 'queued');
