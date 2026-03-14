-- 000160_sport_conflict.up.sql

-- 1. Neues Flag auf activities
ALTER TABLE activities
  ADD COLUMN sport_conflict_pending BOOLEAN NOT NULL DEFAULT false;

-- Partieller Index für Badge-/Count-Abfragen
CREATE INDEX idx_activities_sport_conflict_pending
  ON activities (user_id) WHERE sport_conflict_pending = true;

-- 2. Funktionaler Index für Queue-Lookups (LEFT JOIN LATERAL + Dedup-Check)
CREATE INDEX idx_import_queue_sport_conflict_activity
  ON import_queue ((metadata->>'activity_id'))
  WHERE queue_type = 'sport_conflict' AND status = 'pending';

-- 3. import_queue Constraint erweitern (DROP+ADD in einer Transaktion —
--    go-migrate wraps jede .sql in BEGIN/COMMIT, daher kein explizites BEGIN nötig)
ALTER TABLE import_queue DROP CONSTRAINT IF EXISTS import_queue_queue_type_check;
ALTER TABLE import_queue ADD CONSTRAINT import_queue_queue_type_check
  CHECK (queue_type IN (
    'image_no_exif', 'image_ambiguous', 'image_no_match',
    'pdf_untyped', 'duplicate', 'import_error',
    'sport_conflict'
  ));
