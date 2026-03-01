DROP INDEX IF EXISTS users_legacy_oidc_subject_key;
ALTER TABLE users DROP COLUMN IF EXISTS legacy_oidc_subject;
