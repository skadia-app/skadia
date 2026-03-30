-- Revert: restore "UTC" prefix on fixed-offset timezone strings.
UPDATE activities
SET timezone = regexp_replace(timezone, '^([+-])', 'UTC\1')
WHERE timezone ~ '^[+-][0-9]{2}:[0-9]{2}$';
