-- Soft-Delete für GDPR Account-Löschung (30 Tage Grace Period)
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_users_deleted_at ON users (deleted_at) WHERE deleted_at IS NOT NULL;

-- Force-Password-Change (Admin-Create mit temporärem Passwort)
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false;

-- Email-Änderung: nur pending_email auf User-Zeile.
-- Token-Handling über bestehende email_tokens Tabelle (type='email_change').
ALTER TABLE users ADD COLUMN pending_email TEXT;
