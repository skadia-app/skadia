-- 000232_unified_filestore.up.sql

-- Per-user file naming schemas (one per file type)
CREATE TABLE user_file_schemas (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_type       TEXT NOT NULL,
    schema_template TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, file_type)
);

-- Central file registry
CREATE TABLE user_files (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_type       TEXT NOT NULL,
    storage_backend TEXT NOT NULL,
    storage_path    TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    file_name       TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL DEFAULT 0,
    file_hash       TEXT,
    parent_type     TEXT,
    parent_id       UUID,
    has_sidecar     BOOLEAN NOT NULL DEFAULT false,
    is_renaming     BOOLEAN NOT NULL DEFAULT false,
    status          TEXT NOT NULL DEFAULT 'active',
    replaced_by     UUID REFERENCES user_files(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (storage_backend, storage_path)
);

CREATE INDEX idx_user_files_user_id ON user_files (user_id);
CREATE INDEX idx_user_files_user_type ON user_files (user_id, file_type);
CREATE INDEX idx_user_files_parent ON user_files (parent_type, parent_id) WHERE parent_id IS NOT NULL;
CREATE INDEX idx_user_files_hash ON user_files (user_id, file_hash) WHERE file_hash IS NOT NULL;
CREATE INDEX idx_user_files_status ON user_files (status) WHERE status IN ('archived', 'purging');
CREATE UNIQUE INDEX idx_user_files_edit_upsert ON user_files (user_id, file_type, parent_id) WHERE file_type = 'edit';

-- Multi-parent file linking (bookmarks, not ownership)
CREATE TABLE file_attachments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_id     UUID NOT NULL REFERENCES user_files(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id   UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (file_id, entity_type, entity_id)
);
CREATE INDEX idx_file_attachments_entity ON file_attachments (entity_type, entity_id);
CREATE INDEX idx_file_attachments_user ON file_attachments (user_id);

-- Migration journal (crash-safe file migration tracking)
CREATE TABLE migration_filestore_journal (
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

-- Ownership constraint on file_attachments
CREATE OR REPLACE FUNCTION check_file_attachment_ownership() RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM user_files WHERE id = NEW.file_id AND user_id = NEW.user_id) THEN
        RAISE EXCEPTION 'file_id does not belong to user_id';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_file_attachment_ownership
    BEFORE INSERT ON file_attachments
    FOR EACH ROW EXECUTE FUNCTION check_file_attachment_ownership();

-- file_id columns on domain tables
ALTER TABLE activities ADD COLUMN file_id UUID REFERENCES user_files(id) ON DELETE SET NULL;
ALTER TABLE activities ADD COLUMN sidecar_file_id UUID REFERENCES user_files(id) ON DELETE SET NULL;
ALTER TABLE activity_photos ADD COLUMN file_id UUID REFERENCES user_files(id) ON DELETE SET NULL;
ALTER TABLE gear_documents ADD COLUMN file_id UUID REFERENCES user_files(id) ON DELETE SET NULL;
ALTER TABLE route_photos ADD COLUMN file_id UUID REFERENCES user_files(id) ON DELETE SET NULL;
ALTER TABLE tags ADD COLUMN cover_file_id UUID REFERENCES user_files(id) ON DELETE SET NULL;
ALTER TABLE custom_map_markers ADD COLUMN image_file_id UUID REFERENCES user_files(id) ON DELETE SET NULL;

-- User library directory name (NULL = UUID default)
ALTER TABLE users ADD COLUMN library_dir_name TEXT;
CREATE UNIQUE INDEX idx_users_library_dir_name ON users (library_dir_name) WHERE library_dir_name IS NOT NULL;

-- Migrate existing file_naming_schema to user_file_schemas
-- LTRIM leading '/' — old default from migration 000009 has leading slash
INSERT INTO user_file_schemas (user_id, file_type, schema_template)
SELECT id, 'activity',
    COALESCE(LTRIM(NULLIF(file_naming_schema, ''), '/'), '{year}/{month}/{day}_{time}_{sport}')
FROM users;

-- Insert default schemas for other types (all existing users)
INSERT INTO user_file_schemas (user_id, file_type, schema_template)
SELECT id, 'media', '{year}/{month}/{day}_{title}' FROM users;
INSERT INTO user_file_schemas (user_id, file_type, schema_template)
SELECT id, 'document', '{year}/{category}_{title}' FROM users;
INSERT INTO user_file_schemas (user_id, file_type, schema_template)
SELECT id, 'health', '{year}/{month}/{day}_{type}' FROM users;
INSERT INTO user_file_schemas (user_id, file_type, schema_template)
SELECT id, 'route', '{sport}/{title}' FROM users;
