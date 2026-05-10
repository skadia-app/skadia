-- Add import_type column. NULL = legacy/unknown (UI shows "Unbekannt").
-- DELIBERATELY NO CHECK constraint on import_type — new types should be addable
-- by extending the Go constants in model/import_log.go without a DB migration.
-- Validation happens in Go (NewImportTracker logs a warning for unknown types).
ALTER TABLE import_log ADD COLUMN import_type TEXT;

-- Extend status enum: 'queued' is a terminal status for media files awaiting
-- manual type assignment in import_queue. Excluded from useImportQueue
-- (which polls 'pending,processing'), so the "currently importing" indicator
-- stays clean.
ALTER TABLE import_log DROP CONSTRAINT import_log_status_check;
ALTER TABLE import_log
  ADD CONSTRAINT import_log_status_check
  CHECK (status IN ('pending', 'processing', 'completed', 'skipped', 'error', 'queued'));

-- Composite index for filtered history queries (user_id + import_type).
-- Explicit name makes the down-migration deterministic.
CREATE INDEX import_log_user_id_import_type_idx
  ON import_log (user_id, import_type, created_at DESC);
