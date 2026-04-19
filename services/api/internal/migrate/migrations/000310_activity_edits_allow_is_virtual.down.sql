-- 000310_activity_edits_allow_is_virtual.down.sql
-- FAIL LOUDLY if is_virtual audit rows exist — never silently destroy audit data.

DO $$
DECLARE existing_count INT;
BEGIN
  SELECT COUNT(*) INTO existing_count FROM activity_edits WHERE edit_type = 'is_virtual';
  IF existing_count > 0 THEN
    RAISE EXCEPTION 'cannot drop is_virtual from check: % audit rows still exist. Operator must explicit DELETE them first.', existing_count;
  END IF;
END $$;

ALTER TABLE activity_edits DROP CONSTRAINT IF EXISTS activity_edits_edit_type_check;
ALTER TABLE activity_edits ADD CONSTRAINT activity_edits_edit_type_check
  CHECK (edit_type IN ('trim', 'split', 'fusion_merge'));
