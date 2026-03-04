-- 000065_privacy_zone_polygon.down.sql

DROP INDEX IF EXISTS idx_user_privacy_zones_zone_type;
ALTER TABLE user_privacy_zones
    DROP COLUMN IF EXISTS polygon,
    DROP COLUMN IF EXISTS zone_type;
