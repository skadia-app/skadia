ALTER TABLE users
  ADD COLUMN IF NOT EXISTS password_hash        TEXT,
  ADD COLUMN IF NOT EXISTS email_verified        BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS registration_source   TEXT,
  ADD COLUMN IF NOT EXISTS password_changed_at   TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_login_at         TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS created_by_ip         INET,
  ADD COLUMN IF NOT EXISTS failed_login_count    INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS login_delay_until     TIMESTAMPTZ;
