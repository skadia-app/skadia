-- 000233_filestore_cleanup.up.sql
-- Drops legacy S3/ShadowFS columns now that the Unified FileStore migration is complete.

-- Pre-check: ensure migration is complete
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'migration_filestore_journal') THEN
        IF EXISTS (SELECT 1 FROM migration_filestore_journal WHERE status IN ('pending', 'failed')) THEN
            RAISE EXCEPTION 'filestore migration not complete: pending/failed entries remain in migration_filestore_journal';
        END IF;
    END IF;
END $$;

-- Drop legacy columns from activities
ALTER TABLE activities DROP COLUMN IF EXISTS local_file_path;
ALTER TABLE activities DROP COLUMN IF EXISTS original_file_bucket;
ALTER TABLE activities DROP COLUMN IF EXISTS original_file_key;
ALTER TABLE activities DROP COLUMN IF EXISTS sidecar_meta_key;

-- Drop legacy S3 columns from activity_photos
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_bucket;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_key;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_key_thumb;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_key_preview;

-- Drop legacy S3 columns from gear_documents
ALTER TABLE gear_documents DROP COLUMN IF EXISTS s3_bucket;
ALTER TABLE gear_documents DROP COLUMN IF EXISTS s3_key;
ALTER TABLE gear_documents DROP COLUMN IF EXISTS s3_key_thumb;

-- Drop legacy S3 columns from route_photos
ALTER TABLE route_photos DROP COLUMN IF EXISTS s3_key;
ALTER TABLE route_photos DROP COLUMN IF EXISTS s3_key_thumb;
ALTER TABLE route_photos DROP COLUMN IF EXISTS s3_key_preview;

-- Drop legacy URL columns from tags and custom_map_markers
ALTER TABLE tags DROP COLUMN IF EXISTS cover_image_url;
ALTER TABLE tags DROP COLUMN IF EXISTS cover_image_thumb_url;
ALTER TABLE custom_map_markers DROP COLUMN IF EXISTS image_url;

-- Drop legacy file naming schema column from users (migrated to user_file_schemas in 000232)
ALTER TABLE users DROP COLUMN IF EXISTS file_naming_schema;

-- Drop legacy S3 staging columns from import_queue
ALTER TABLE import_queue DROP COLUMN IF EXISTS s3_bucket;
ALTER TABLE import_queue DROP COLUMN IF EXISTS s3_key;

-- Drop migration journal now that cleanup is done
DROP TABLE IF EXISTS migration_filestore_journal;
