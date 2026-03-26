-- 000236_drop_legacy_storage_columns.up.sql
--
-- Drop legacy S3/file-path columns that have been replaced by the unified
-- user_files registry (file_id FK on each domain table, migration 000232).
--
-- SAFETY: This migration is irreversible. All data in these columns is lost.
-- Ensure ALL of the following before running:
--   1. user_files has entries (SELECT COUNT(*) FROM user_files > 0)
--   2. No Go code references these columns (grep returns 0)
--   3. All activities with files have file_id set
--   4. All photos with files have file_id set

-- ── activities ────────────────────────────────────────────────────────────────
ALTER TABLE activities DROP COLUMN IF EXISTS local_file_path;
ALTER TABLE activities DROP COLUMN IF EXISTS original_file_key;
ALTER TABLE activities DROP COLUMN IF EXISTS original_file_bucket;
ALTER TABLE activities DROP COLUMN IF EXISTS sidecar_meta_key;

-- ── activity_photos ──────────────────────────────────────────────────────────
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_bucket;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_key;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_key_thumb;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS s3_key_preview;

-- ── route_photos ─────────────────────────────────────────────────────────────
ALTER TABLE route_photos DROP COLUMN IF EXISTS s3_key;
ALTER TABLE route_photos DROP COLUMN IF EXISTS s3_key_thumb;
ALTER TABLE route_photos DROP COLUMN IF EXISTS s3_key_preview;

-- ── gear_documents ───────────────────────────────────────────────────────────
ALTER TABLE gear_documents DROP COLUMN IF EXISTS s3_bucket;
ALTER TABLE gear_documents DROP COLUMN IF EXISTS s3_key;
ALTER TABLE gear_documents DROP COLUMN IF EXISTS s3_key_thumb;

-- ── tags ─────────────────────────────────────────────────────────────────────
ALTER TABLE tags DROP COLUMN IF EXISTS cover_image_url;
ALTER TABLE tags DROP COLUMN IF EXISTS cover_image_thumb_url;

-- ── custom_map_markers ───────────────────────────────────────────────────────
ALTER TABLE custom_map_markers DROP COLUMN IF EXISTS image_url;

-- ── users ────────────────────────────────────────────────────────────────────
ALTER TABLE users DROP COLUMN IF EXISTS file_naming_schema;

-- ── migration journal (one-time migration tracking, no longer needed) ────────
DROP TABLE IF EXISTS migration_shadowfs_journal;
