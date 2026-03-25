DROP INDEX IF EXISTS idx_users_deleted_at;
ALTER TABLE users DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE users DROP COLUMN IF EXISTS must_change_password;
ALTER TABLE users DROP COLUMN IF EXISTS pending_email;
