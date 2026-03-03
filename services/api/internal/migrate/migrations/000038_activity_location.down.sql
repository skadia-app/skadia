ALTER TABLE activities
    DROP COLUMN IF EXISTS location_lat,
    DROP COLUMN IF EXISTS location_lng;
