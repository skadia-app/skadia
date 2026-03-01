-- OIDC-Subjekt-ID für OIDC-Login
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS legacy_oidc_subject TEXT UNIQUE;

-- Unique-Constraint für ON CONFLICT in GetOrCreateUserByOIDCSubject
CREATE UNIQUE INDEX IF NOT EXISTS users_legacy_oidc_subject_key ON users (legacy_oidc_subject)
    WHERE legacy_oidc_subject IS NOT NULL;
