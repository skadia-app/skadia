-- 000065_privacy_zone_polygon.up.sql
-- Polygon-Unterstützung für Private Zonen

ALTER TABLE user_privacy_zones
    ADD COLUMN IF NOT EXISTS zone_type TEXT NOT NULL DEFAULT 'circle',
    ADD COLUMN IF NOT EXISTS polygon   GEOMETRY(Polygon, 4326);

-- Alle bestehenden Zonen sind Kreise
UPDATE user_privacy_zones SET zone_type = 'circle' WHERE zone_type IS NULL OR zone_type = '';

CREATE INDEX IF NOT EXISTS idx_user_privacy_zones_zone_type ON user_privacy_zones (zone_type);
