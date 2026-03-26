ALTER TABLE import_queue ADD COLUMN IF NOT EXISTS s3_bucket TEXT;
ALTER TABLE import_queue ADD COLUMN IF NOT EXISTS s3_key TEXT;
UPDATE import_queue SET s3_key = storage_path WHERE storage_backend = 's3';
ALTER TABLE import_queue DROP COLUMN IF EXISTS storage_backend;
ALTER TABLE import_queue DROP COLUMN IF EXISTS storage_path;
