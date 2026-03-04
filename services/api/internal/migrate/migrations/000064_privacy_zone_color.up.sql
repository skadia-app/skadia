-- 000064_privacy_zone_color.up.sql

-- Farbfeld für Private Zonen (hex-Farbe, z.B. #F97316)
ALTER TABLE user_privacy_zones ADD COLUMN IF NOT EXISTS color TEXT NOT NULL DEFAULT '#F97316';
