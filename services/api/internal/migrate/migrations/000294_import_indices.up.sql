CREATE INDEX IF NOT EXISTS idx_user_files_hash_user
    ON user_files (user_id, file_hash) WHERE file_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tags_user_name
    ON tags (user_id, name);

CREATE INDEX IF NOT EXISTS idx_gear_user_name_type
    ON gear (user_id, name, type);
