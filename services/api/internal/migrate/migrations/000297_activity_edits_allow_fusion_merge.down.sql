-- Rollback: entferne 'fusion_merge' aus erlaubten edit_types.
-- ACHTUNG: Schlägt fehl, wenn bereits Rows mit edit_type='fusion_merge' existieren.

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;

ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
    CHECK (edit_type IN ('trim', 'split', 'merge'));
