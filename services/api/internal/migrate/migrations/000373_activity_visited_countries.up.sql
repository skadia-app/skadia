ALTER TABLE activities
  ADD COLUMN IF NOT EXISTS end_country TEXT,
  ADD COLUMN IF NOT EXISTS end_country_code TEXT,
  ADD COLUMN IF NOT EXISTS location_country TEXT,
  ADD COLUMN IF NOT EXISTS location_country_code TEXT,
  ADD COLUMN IF NOT EXISTS visited_country_codes TEXT[] NOT NULL DEFAULT '{}';

UPDATE activities
SET visited_country_codes = ARRAY(
  SELECT DISTINCT upper(trim(code)) AS normalized_code
  FROM unnest(ARRAY[start_country_code, end_country_code, location_country_code]) AS code
  WHERE code IS NOT NULL AND length(trim(code)) = 2
  ORDER BY normalized_code
)
WHERE visited_country_codes = '{}';

CREATE INDEX IF NOT EXISTS activities_visited_country_codes_gin_idx
  ON activities USING GIN (visited_country_codes);
