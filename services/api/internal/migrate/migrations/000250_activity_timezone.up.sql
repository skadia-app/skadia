ALTER TABLE activities ADD COLUMN IF NOT EXISTS timezone TEXT;

COMMENT ON COLUMN activities.timezone IS 'IANA timezone string (e.g. Europe/Berlin) determined from GPS start position via tzf library. NULL for indoor activities without GPS.';
