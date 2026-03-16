ALTER TABLE import_log
  ADD CONSTRAINT import_log_status_check
  CHECK (status IN ('pending', 'processing', 'completed', 'skipped', 'error'));
