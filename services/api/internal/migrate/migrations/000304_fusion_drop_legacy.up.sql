-- ⚠️  RUNBOOK NOTICE ⚠️
-- This migration DROPS the legacy is_fusion_source + fusion_sources columns.
-- After this runs, any rollback via the .down.sql MUST happen within the
-- same deployment window (same day ideally). New memberships with multi-source
-- per-field-key — semantically unrepresentable in the legacy schema —
-- collapse to the literal "fuse" on rollback and lose source-id precision.
--
-- Rollback decision tree:
--   - Same deployment, no new merges happened → down is safe.
--   - Production traffic post-migration with new merges → down is LOSSY.
--     Prefer forward-fix over rollback. Document the data-loss risk to
--     stakeholders before running down.sql.

-- Phase 3 Task 26 — drop legacy fusion columns + V1-parent hiding cleanup.
-- Prerequisite: All production reads migrated to activity_fusion_memberships
-- (Phases 2a-2d). Dual-Write removed in Task 27 (follow-up).
--
-- Dev-DB estimate (2026-04-16): 0 rows with fusion_sources IS NOT NULL,
-- activities table ~108 MB. Direct DROP is safe (no lock-time concern).
-- Production runbook: spec §8 Phase 3 — if table size > 200 MB × col size,
-- prefer two-step drop (NULL → rewrite on off-hour → drop).

-- 1. V1-parent_id hiding cleanup. V1 FusionMerge set sources' parent_id to
--    the merged_id to hide them from the activity list. Multisport parents
--    have sport_type='multisport' and must keep their parent links.
UPDATE activities SET parent_id = NULL
WHERE parent_id IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM activities p
    WHERE p.id = activities.parent_id
      AND p.sport_type != 'multisport'
  );

-- 2. Drop the old UPDATE trigger — its WHEN-clause references the columns
--    we're about to drop.
DROP TRIGGER IF EXISTS trg_fusion_cache_upd ON activities;

-- 3. Recreate without the obsolete columns. The reduced WHEN-clause keeps
--    bumping for all columns that actually affect fusion-candidate eligibility:
--    started_at, ended_at, has_gps. Other columns (title, description,
--    sport_type, etc.) do NOT participate in fusion-candidate scoring
--    (see handler/fusion.go CheckTier2 — time-range + gps overlap only).
CREATE TRIGGER trg_fusion_cache_upd
    AFTER UPDATE ON activities
    FOR EACH ROW
    WHEN (OLD.started_at IS DISTINCT FROM NEW.started_at
       OR OLD.ended_at IS DISTINCT FROM NEW.ended_at
       OR OLD.has_gps IS DISTINCT FROM NEW.has_gps)
    EXECUTE FUNCTION invalidate_fusion_cache();

-- 4. Safe to drop the columns now.
ALTER TABLE activities DROP COLUMN IF EXISTS is_fusion_source;
ALTER TABLE activities DROP COLUMN IF EXISTS fusion_sources;
