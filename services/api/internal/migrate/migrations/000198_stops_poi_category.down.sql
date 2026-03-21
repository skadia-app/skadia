ALTER TABLE activity_stops
    DROP COLUMN IF EXISTS poi_category,
    DROP COLUMN IF EXISTS confidence;
