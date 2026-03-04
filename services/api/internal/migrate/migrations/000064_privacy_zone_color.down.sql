-- 000064_privacy_zone_color.down.sql

ALTER TABLE user_privacy_zones DROP COLUMN IF EXISTS color;
