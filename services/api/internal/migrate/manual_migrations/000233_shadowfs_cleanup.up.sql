-- 000233_shadowfs_cleanup.up.sql
-- Drops legacy S3/ShadowFS columns now that the Unified ShadowFS migration is complete.

-- Pre-check: STRICT guard — this migration must NEVER run automatically.
-- It requires ALL files to be migrated to ShadowFS registry first.
DO $$
BEGIN
    -- Guard 1: Any activities without file_id but with legacy file references?
    IF EXISTS (SELECT 1 FROM activities WHERE file_id IS NULL AND (local_file_path IS NOT NULL OR original_file_key IS NOT NULL)) THEN
        RAISE EXCEPTION 'shadowfs cleanup blocked: % activities have legacy file refs but no file_id — run registry population first',
            (SELECT COUNT(*) FROM activities WHERE file_id IS NULL AND (local_file_path IS NOT NULL OR original_file_key IS NOT NULL));
    END IF;
    -- Guard 2: Any photos without file_id but with legacy S3 keys?
    IF EXISTS (SELECT 1 FROM activity_photos WHERE file_id IS NULL AND s3_key IS NOT NULL) THEN
        RAISE EXCEPTION 'shadowfs cleanup blocked: % photos have legacy S3 keys but no file_id',
            (SELECT COUNT(*) FROM activity_photos WHERE file_id IS NULL AND s3_key IS NOT NULL);
    END IF;
    -- Guard 3: Journal has pending/failed entries?
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'migration_shadowfs_journal') THEN
        IF EXISTS (SELECT 1 FROM migration_shadowfs_journal WHERE status IN ('pending', 'failed')) THEN
            RAISE EXCEPTION 'shadowfs cleanup blocked: pending/failed entries remain in migration_shadowfs_journal';
        END IF;
    END IF;
    -- Guard 4: File move job still running?
    IF EXISTS (SELECT 1 FROM background_jobs WHERE job_type IN ('shadowfs_file_move', 'shadowfs_rollback') AND status = 'running') THEN
        RAISE EXCEPTION 'shadowfs cleanup blocked: file move job is still running';
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
DROP TABLE IF EXISTS migration_shadowfs_journal;
