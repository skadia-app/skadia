ALTER TABLE activity_geo_tags
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS admin_level;
