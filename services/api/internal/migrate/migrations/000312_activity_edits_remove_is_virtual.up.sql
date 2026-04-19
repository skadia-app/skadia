-- Reverts 000310 (is_virtual edit_type addition). The is_virtual-toggle
-- feature was removed — is_virtual is an import-time FIT property, not
-- user-toggleable. Any remaining is_virtual audit rows are test artifacts
-- from pre-revert smoke-tests.

DO $$
DECLARE
    n INTEGER;
BEGIN
    SELECT COUNT(*) INTO n FROM activity_edits WHERE edit_type = 'is_virtual';
    IF n > 0 THEN
        RAISE NOTICE 'Migration 000312: removing % activity_edits rows with edit_type = is_virtual', n;
    END IF;
END $$;

DELETE FROM activity_edits WHERE edit_type = 'is_virtual';

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge'));
