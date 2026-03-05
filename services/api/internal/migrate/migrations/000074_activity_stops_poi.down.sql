ALTER TABLE activity_stops
    DROP COLUMN IF EXISTS poi_type,
    DROP COLUMN IF EXISTS poi_name,
    DROP COLUMN IF EXISTS poi_osm_id;
