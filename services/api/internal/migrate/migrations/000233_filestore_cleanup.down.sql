-- 000233_filestore_cleanup.down.sql
-- Restores legacy S3/ShadowFS columns as nullable (no data recovery possible).

-- Recreate migration journal so 000232 down-migration can work correctly.
CREATE TABLE IF NOT EXISTS migration_shadowfs_journal (
    source_table TEXT NOT NULL,
    source_id    UUID NOT NULL,
    phase        TEXT NOT NULL DEFAULT 'registry',
    status       TEXT NOT NULL DEFAULT 'pending',
    user_id      UUID,
    file_id      UUID,
    old_path     TEXT,
    new_path     TEXT,
    error_msg    TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_table, source_id, phase)
);

-- Restore legacy columns on activities
ALTER TABLE activities ADD COLUMN IF NOT EXISTS local_file_path TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS original_file_bucket TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS original_file_key TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS sidecar_meta_key TEXT;

-- Restore legacy S3 columns on activity_photos
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_bucket TEXT;
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_key TEXT;
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_key_thumb TEXT;
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_key_preview TEXT;

-- Restore legacy S3 columns on gear_documents
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS s3_bucket TEXT;
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS s3_key TEXT;
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS s3_key_thumb TEXT;

-- Restore legacy S3 columns on route_photos
ALTER TABLE route_photos ADD COLUMN IF NOT EXISTS s3_key TEXT;
ALTER TABLE route_photos ADD COLUMN IF NOT EXISTS s3_key_thumb TEXT;
ALTER TABLE route_photos ADD COLUMN IF NOT EXISTS s3_key_preview TEXT;

-- Restore legacy URL columns on tags and custom_map_markers
ALTER TABLE tags ADD COLUMN IF NOT EXISTS cover_image_url TEXT;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS cover_image_thumb_url TEXT;
ALTER TABLE custom_map_markers ADD COLUMN IF NOT EXISTS image_url TEXT;

-- Restore file_naming_schema on users (data migrated to user_file_schemas in 000232)
ALTER TABLE users ADD COLUMN IF NOT EXISTS file_naming_schema TEXT;
UPDATE users SET file_naming_schema = (
    SELECT schema_template FROM user_file_schemas
    WHERE user_file_schemas.user_id = users.id AND file_type = 'activity'
);

-- Restore legacy S3 staging columns on import_queue
ALTER TABLE import_queue ADD COLUMN IF NOT EXISTS s3_bucket TEXT;
ALTER TABLE import_queue ADD COLUMN IF NOT EXISTS s3_key TEXT;
