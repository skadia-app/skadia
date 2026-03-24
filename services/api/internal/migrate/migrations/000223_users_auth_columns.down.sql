ALTER TABLE users
  DROP COLUMN IF EXISTS login_delay_until,
  DROP COLUMN IF EXISTS failed_login_count,
  DROP COLUMN IF EXISTS created_by_ip,
  DROP COLUMN IF EXISTS last_login_at,
  DROP COLUMN IF EXISTS password_changed_at,
  DROP COLUMN IF EXISTS registration_source,
  DROP COLUMN IF EXISTS email_verified,
  DROP COLUMN IF EXISTS password_hash;
