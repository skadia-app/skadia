DROP INDEX IF EXISTS import_log_user_id_import_type_idx;

-- Restore the original status CHECK (without 'queued').
-- Any rows with status='queued' would block the constraint; clear them first.
UPDATE import_log SET status = 'pending' WHERE status = 'queued';
ALTER TABLE import_log DROP CONSTRAINT IF EXISTS import_log_status_check;
ALTER TABLE import_log
  ADD CONSTRAINT import_log_status_check
  CHECK (status IN ('pending', 'processing', 'completed', 'skipped', 'error'));

ALTER TABLE import_log DROP COLUMN IF EXISTS import_type;
