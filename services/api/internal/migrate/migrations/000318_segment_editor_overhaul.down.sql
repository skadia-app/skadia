-- Rollback for migration 000318 (Segment-Editor-Overhaul).
--
-- Irreversible parts: surface-label and difficulty_color normalizations stay
-- on the new keys. Full restore requires backup. This only rolls back schema
-- changes (constraint + column + NOT NULL).
--
-- Pre-000318 state had NO CHECK constraint on segment_type at all (verified
-- 2026-04-20 via pg_constraint against dev DB; 000032_segments.up.sql defines
-- the column without a CHECK, and no later migration adds one). We therefore
-- roll back to the true pre-state by dropping the constraint entirely rather
-- than installing a synthetic restrictive one — the latter would paradoxically
-- block wintersport-rows that existed before the rollback ran.
--
-- segment_type_meta NOT NULL and DEFAULT '{}' are also retained on rollback:
-- these are conservative hardening changes that do no harm in the pre-000318
-- schema and avoid a potentially unsafe ALTER that could fail if new NULL rows
-- were somehow inserted during a failed partial migration.

ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_segment_type_check;
ALTER TABLE segments DROP COLUMN IF EXISTS bidirectional;
