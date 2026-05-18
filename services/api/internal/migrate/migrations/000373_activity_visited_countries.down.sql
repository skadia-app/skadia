DROP INDEX IF EXISTS activities_visited_country_codes_gin_idx;

ALTER TABLE activities
  DROP COLUMN IF EXISTS visited_country_codes,
  DROP COLUMN IF EXISTS location_country_code,
  DROP COLUMN IF EXISTS location_country,
  DROP COLUMN IF EXISTS end_country_code,
  DROP COLUMN IF EXISTS end_country;
