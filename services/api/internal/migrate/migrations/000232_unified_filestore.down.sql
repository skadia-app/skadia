-- 000232_unified_filestore.down.sql

-- Reverse-copy activity schemas back to users.file_naming_schema
UPDATE users SET file_naming_schema = (
    SELECT schema_template FROM user_file_schemas
    WHERE user_file_schemas.user_id = users.id AND file_type = 'activity'
);

ALTER TABLE custom_map_markers DROP COLUMN IF EXISTS image_file_id;
ALTER TABLE tags DROP COLUMN IF EXISTS cover_file_id;
ALTER TABLE route_photos DROP COLUMN IF EXISTS file_id;
ALTER TABLE gear_documents DROP COLUMN IF EXISTS file_id;
ALTER TABLE activity_photos DROP COLUMN IF EXISTS file_id;
ALTER TABLE activities DROP COLUMN IF EXISTS sidecar_file_id;
ALTER TABLE activities DROP COLUMN IF EXISTS file_id;

DROP INDEX IF EXISTS idx_users_library_dir_name;
ALTER TABLE users DROP COLUMN IF EXISTS library_dir_name;

DROP TRIGGER IF EXISTS trg_file_attachment_ownership ON file_attachments;
DROP FUNCTION IF EXISTS check_file_attachment_ownership();

DROP TABLE IF EXISTS migration_filestore_journal;
DROP TABLE IF EXISTS file_attachments;
DROP TABLE IF EXISTS user_files;
DROP TABLE IF EXISTS user_file_schemas;
