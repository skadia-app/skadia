-- 000310_activity_edits_allow_is_virtual.up.sql
-- Extends the activity_edits.edit_type CHECK to include 'is_virtual' audit rows,
-- written by the SetActivityVirtual handler.

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge', 'is_virtual'));
