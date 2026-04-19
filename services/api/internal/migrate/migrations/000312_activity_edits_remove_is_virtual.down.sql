-- Re-allow is_virtual (mirrors 000310.up.sql). Does NOT restore deleted
-- audit rows (forward-only).

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge', 'is_virtual'));
