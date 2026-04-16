BEGIN;
DROP INDEX IF EXISTS idx_activity_photos_source_activity_id;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS source_activity_id;
COMMIT;
