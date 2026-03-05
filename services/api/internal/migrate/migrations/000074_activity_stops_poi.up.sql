ALTER TABLE activity_stops
    ADD COLUMN IF NOT EXISTS poi_type TEXT,
    ADD COLUMN IF NOT EXISTS poi_name TEXT,
    ADD COLUMN IF NOT EXISTS poi_osm_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_activity_stops_poi_osm_id ON activity_stops(poi_osm_id) WHERE poi_osm_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_activity_stops_poi_type ON activity_stops(poi_type) WHERE poi_type IS NOT NULL;
