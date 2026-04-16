-- 🛑 DO NOT ROLL BACK WITHOUT REVIEW 🛑
-- See the up-migration's RUNBOOK NOTICE. This script is lossy for
-- multi-source-per-field-key memberships created after the up-migration ran.
-- Before executing:
--   1. Confirm same deployment window OR accept data-loss.
--   2. Back up activity_fusion_memberships first (pg_dump -t activity_fusion_memberships).
--   3. Approve with tech lead.

-- Down for migration 000304.
--
-- WARNING — NOT fully lossless (Finding #22):
--  - Pre-302 fusion_sources JSONB allowed exactly ONE source per field_key
--    (or the literal "fuse" marker for multi-source stitching).
--  - Memberships allow MULTIPLE sources per field_key (needed for new semantics).
--  - During rollback, any field_key with >1 source is written back as the
--    literal "fuse" — even when the original intent was two concrete source_ids
--    (a pattern the old schema could NOT represent).
--  - Therefore: forward → down → forward can lose information for any
--    non-"fuse" multi-source field_keys. Historically none exist (old code
--    only ever wrote single source_ids or "fuse"), so a same-day rollback
--    is safe.
--
-- Rollback-ordering rule (Finding #33): if 000304 is to roll back AND 000302
-- (memberships) is to roll back, always roll 000304 down FIRST. Otherwise
-- the UPDATE trigger would reference a dropped function.

BEGIN;

ALTER TABLE activities ADD COLUMN IF NOT EXISTS is_fusion_source BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS fusion_sources JSONB;

DROP TRIGGER IF EXISTS trg_fusion_cache_upd ON activities;
CREATE TRIGGER trg_fusion_cache_upd
    AFTER UPDATE ON activities
    FOR EACH ROW
    WHEN (OLD.started_at IS DISTINCT FROM NEW.started_at
       OR OLD.ended_at IS DISTINCT FROM NEW.ended_at
       OR OLD.has_gps IS DISTINCT FROM NEW.has_gps
       OR OLD.is_fusion_source IS DISTINCT FROM NEW.is_fusion_source
       OR OLD.fusion_sources IS DISTINCT FROM NEW.fusion_sources)
    EXECUTE FUNCTION invalidate_fusion_cache();

-- Best-effort recovery from memberships (fuse-literal cannot be restored —
-- multi-source field_keys collapse to the string "fuse").
UPDATE activities a SET fusion_sources = sub.js, is_fusion_source = FALSE
FROM (
  SELECT merged_id,
         jsonb_object_agg(field_key,
           CASE WHEN cnt > 1 THEN '"fuse"'::jsonb ELSE to_jsonb(source_id::text) END) AS js
  FROM (
    SELECT merged_id, field_key, source_id,
           COUNT(*) OVER (PARTITION BY merged_id, field_key) AS cnt
    FROM activity_fusion_memberships
  ) t
  GROUP BY merged_id
) sub
WHERE a.id = sub.merged_id;

UPDATE activities a SET is_fusion_source = TRUE
WHERE EXISTS (SELECT 1 FROM activity_fusion_memberships m WHERE m.source_id = a.id);

COMMIT;
