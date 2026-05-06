-- Reverse Phase 4c migration. Drops the index, the audit flags, and the
-- source/segment_effort_id columns. Symmetric to the up.sql.

DROP INDEX IF EXISTS idx_alc_heuristic_only_confirmed;

ALTER TABLE segment_efforts DROP COLUMN IF EXISTS is_cable_hold_merge;
ALTER TABLE segment_efforts DROP COLUMN IF EXISTS is_method_a_only;
ALTER TABLE segment_efforts DROP COLUMN IF EXISTS is_lift_segment_match;

ALTER TABLE activity_lift_candidates DROP COLUMN IF EXISTS segment_effort_id;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'activity_lift_candidates_source_check'
    ) THEN
        ALTER TABLE activity_lift_candidates
            DROP CONSTRAINT activity_lift_candidates_source_check;
    END IF;
END
$$;

ALTER TABLE activity_lift_candidates DROP COLUMN IF EXISTS source;
