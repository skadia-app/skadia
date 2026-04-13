CREATE INDEX IF NOT EXISTS idx_user_files_hash_user
    ON user_files (user_id, file_hash) WHERE file_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_tags_user_name
    ON tags (user_id, name);

CREATE INDEX IF NOT EXISTS idx_gear_user_name_type
    ON gear (user_id, name, type);

-- Fix invalidate_fusion_cache: affected_user must be UUID, not TEXT,
-- to allow the uuid = uuid comparison in the WHERE clause (SQLSTATE 42883).
CREATE OR REPLACE FUNCTION invalidate_fusion_cache() RETURNS trigger AS $$
DECLARE
    affected_user UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_user := OLD.user_id;
    ELSE
        affected_user := NEW.user_id;
    END IF;
    UPDATE users SET fusion_invalidation_seq = fusion_invalidation_seq + 1
    WHERE id = affected_user;
    IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END;
$$ LANGUAGE plpgsql;
