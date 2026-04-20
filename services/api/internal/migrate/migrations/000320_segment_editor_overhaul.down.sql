-- Rollback for migration 000320 (Segment-Editor-Overhaul).
--
-- Irreversible parts: surface-label and difficulty_color normalizations stay
-- on the new keys. Full restore requires backup. This only rolls back schema
-- changes (constraint + column).
--
-- Pre-000320 state had NO CHECK constraint on segment_type at all (verified
-- 2026-04-20 via pg_constraint against dev DB; 000032_segments.up.sql defines
-- the column without a CHECK, and no later migration adds one). We therefore
-- roll back to the true pre-state by dropping the constraint entirely rather
-- than installing a synthetic restrictive one — the latter would paradoxically
-- block wintersport-rows that existed before the rollback ran.
--
-- segment_type_meta DEFAULT '{}' is retained on rollback (harmless hardening).
-- No NOT NULL constraint was added by this migration (Batch-6B finding #4:
-- NOT NULL was removed from up-migration as out-of-scope per Spec §4.9), so
-- nothing to reverse on that front.

ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_segment_type_check;
ALTER TABLE segments DROP COLUMN IF EXISTS bidirectional;
