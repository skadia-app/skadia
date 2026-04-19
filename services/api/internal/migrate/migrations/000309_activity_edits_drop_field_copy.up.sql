-- 000309_activity_edits_drop_field_copy.up.sql
-- Removes the 'field_copy' value from the edit_type check constraint.
-- Rationale: CopyFields feature deprecated; UI was removed in commit 1bea6c6f,
-- Handler removed in companion commit 205c245f. See fusion consistency
-- refactor (2026-04-16) — fusion memberships are the single source of truth
-- for cross-activity field provenance.
--
-- Pre-deployment check: zero field_copy rows in production (verified 2026-04-19).
-- No DELETE needed; constraint tightening is sufficient.
-- Safety-net: migration aborts loudly if any field_copy audit rows still exist,
-- so operators can migrate/delete them before re-running. See 000308 for the
-- same pattern applied to the legacy 'merge' edit_type.

BEGIN;

-- Safety-net: abort if any field_copy rows still exist (e.g. on selfhosted
-- instances that ran CopyFields before the UI was removed). Operator action:
-- DELETE FROM activity_edits WHERE edit_type = 'field_copy'; then re-run.
DO $$
DECLARE
    field_copy_count INT;
BEGIN
    SELECT COUNT(*) INTO field_copy_count FROM activity_edits WHERE edit_type = 'field_copy';
    IF field_copy_count > 0 THEN
        RAISE EXCEPTION
            'cannot drop field_copy from check: % rows still use edit_type=''field_copy''. '
            'Run: DELETE FROM activity_edits WHERE edit_type = ''field_copy''; then re-run migration.',
            field_copy_count;
    END IF;
END $$;

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge'));

COMMIT;
