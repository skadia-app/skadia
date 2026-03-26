-- 000236_drop_legacy_storage_columns.down.sql
--
-- Re-add legacy columns. Data is NOT recoverable — columns will be empty.

ALTER TABLE activities ADD COLUMN IF NOT EXISTS local_file_path TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS original_file_key TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS original_file_bucket TEXT;
ALTER TABLE activities ADD COLUMN IF NOT EXISTS sidecar_meta_key TEXT;

ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_bucket TEXT;
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_key TEXT;
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_key_thumb TEXT;
ALTER TABLE activity_photos ADD COLUMN IF NOT EXISTS s3_key_preview TEXT;

ALTER TABLE route_photos ADD COLUMN IF NOT EXISTS s3_key TEXT;
ALTER TABLE route_photos ADD COLUMN IF NOT EXISTS s3_key_thumb TEXT;
ALTER TABLE route_photos ADD COLUMN IF NOT EXISTS s3_key_preview TEXT;

ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS s3_bucket TEXT;
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS s3_key TEXT;
ALTER TABLE gear_documents ADD COLUMN IF NOT EXISTS s3_key_thumb TEXT;

ALTER TABLE tags ADD COLUMN IF NOT EXISTS cover_image_url TEXT;
ALTER TABLE tags ADD COLUMN IF NOT EXISTS cover_image_thumb_url TEXT;

ALTER TABLE custom_map_markers ADD COLUMN IF NOT EXISTS image_url TEXT;

ALTER TABLE users ADD COLUMN IF NOT EXISTS file_naming_schema TEXT;

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
