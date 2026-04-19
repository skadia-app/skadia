-- 000309_activity_edits_drop_field_copy.up.sql
-- Removes the 'field_copy' value from the edit_type check constraint.
-- Rationale: CopyFields feature deprecated; UI was removed in commit 1bea6c6f,
-- Handler removed in companion commit 205c245f. See fusion consistency
-- refactor (2026-04-16) — fusion memberships are the single source of truth
-- for cross-activity field provenance.
--
-- Pre-deployment check: zero field_copy rows in production (verified 2026-04-19).
-- No DELETE needed; constraint tightening is sufficient.

BEGIN;

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge'));

COMMIT;
