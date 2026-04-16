-- Phase 2c Task 20 — allow edit_type='field_copy' in activity_edits.
-- Baseline constraint (mig 000297): CHECK (edit_type IN ('trim', 'split', 'merge', 'fusion_merge')).
BEGIN;
ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'merge', 'fusion_merge', 'field_copy'));
COMMIT;
