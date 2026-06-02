ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_units_supported_chk;

ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_locale_supported_chk;
