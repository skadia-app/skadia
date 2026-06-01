UPDATE users
SET locale = 'de'
WHERE locale IS NULL OR locale NOT IN ('de', 'en', 'fr', 'it', 'es');

UPDATE users
SET units = 'metric'
WHERE units IS NULL OR units NOT IN ('metric', 'imperial');

ALTER TABLE users
  ADD CONSTRAINT users_locale_supported_chk
  CHECK (locale IN ('de', 'en', 'fr', 'it', 'es'));

ALTER TABLE users
  ADD CONSTRAINT users_units_supported_chk
  CHECK (units IN ('metric', 'imperial'));
