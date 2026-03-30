-- Fix activities that have "UTC+HH:MM" or "UTC-HH:MM" timezone values.
-- These were written by an older version of the enrichment code (before b2f4b2f6).
-- The Intl.DateTimeFormat API rejects the "UTC" prefix with a RangeError.
-- Correct format is "+HH:MM" / "-HH:MM".
UPDATE activities
SET timezone = regexp_replace(timezone, '^UTC([+-])', '\1')
WHERE timezone ~ '^UTC[+-]';
