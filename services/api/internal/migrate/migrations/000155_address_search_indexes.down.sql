-- Nur osm_ways-Indexes droppen — osm_pois-Index gehört zu 000151
DROP INDEX IF EXISTS idx_osm_ways_name_trgm;
DROP INDEX IF EXISTS idx_osm_ways_highway_name;
-- idx_osm_pois_name_trgm bleibt (gehört zu 000151)
