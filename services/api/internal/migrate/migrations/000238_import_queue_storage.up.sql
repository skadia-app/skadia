ALTER TABLE import_queue ADD COLUMN IF NOT EXISTS storage_backend TEXT;
ALTER TABLE import_queue ADD COLUMN IF NOT EXISTS storage_path TEXT;
UPDATE import_queue SET storage_backend = 's3', storage_path = s3_key WHERE s3_key IS NOT NULL;
UPDATE import_queue SET storage_backend = 'local' WHERE s3_key IS NULL AND storage_backend IS NULL;
ALTER TABLE import_queue DROP COLUMN IF EXISTS s3_bucket;
ALTER TABLE import_queue DROP COLUMN IF EXISTS s3_key;
