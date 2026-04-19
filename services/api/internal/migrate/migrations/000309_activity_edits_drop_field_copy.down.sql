-- 000309_activity_edits_drop_field_copy.down.sql
-- Reverts constraint to allow field_copy again.
-- Does NOT restore deleted audit rows (there were none at time of migration).

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge', 'field_copy'));
