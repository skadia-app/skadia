-- Phase 5 follow-up — reduce edit_type CHECK constraint.
-- The 'merge' edit_type was renamed to 'field_copy' in Phase 2c (commit 5da4e7d7).
-- Dev-DB verified: zero rows with edit_type='merge'. If any exist in production
-- at rollout, migrate them to 'field_copy' before running this migration.
BEGIN;

-- Safety-net: abort if any legacy 'merge' rows still exist.
DO $$
DECLARE
    legacy_count INT;
BEGIN
    SELECT COUNT(*) INTO legacy_count FROM activity_edits WHERE edit_type = 'merge';
    IF legacy_count > 0 THEN
        RAISE EXCEPTION 'cannot drop merge from check: % rows still use legacy edit_type', legacy_count;
    END IF;
END $$;

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge', 'field_copy'));

COMMIT;
