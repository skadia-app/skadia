ALTER TABLE activity_geo_tags
    ADD COLUMN IF NOT EXISTS map_type TEXT,
    ADD COLUMN IF NOT EXISTS map_point_lat DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS map_point_lon DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS map_bbox_west DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS map_bbox_south DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS map_bbox_east DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS map_bbox_north DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS map_section GEOMETRY(LineString, 4326),
    ADD COLUMN IF NOT EXISTS length_m DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS ele_m DOUBLE PRECISION;

-- Index für Karten-Abfragen
CREATE INDEX IF NOT EXISTS idx_activity_geo_tags_category ON activity_geo_tags(activity_id, category);
-- Spatial index für map_section (Grenzüberquerungen)
CREATE INDEX IF NOT EXISTS idx_activity_geo_tags_map_section ON activity_geo_tags USING GIST(map_section) WHERE map_section IS NOT NULL;
