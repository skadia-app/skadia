-- Down: fails if any row with edit_type='field_copy' still exists.
-- Delete those rows first or accept the failure.
BEGIN;
ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'merge', 'fusion_merge'));
COMMIT;
