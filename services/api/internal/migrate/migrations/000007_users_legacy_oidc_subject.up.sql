-- Legacy OIDC subject for historical login data.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS legacy_oidc_subject TEXT UNIQUE;

CREATE UNIQUE INDEX IF NOT EXISTS users_legacy_oidc_subject_key ON users (legacy_oidc_subject)
    WHERE legacy_oidc_subject IS NOT NULL;
