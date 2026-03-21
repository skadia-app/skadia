-- 000195_osm_ways_dedup_osm_id.down.sql
DROP INDEX IF EXISTS idx_osm_ways_osm_id;
CREATE INDEX idx_osm_ways_osm_id ON osm_ways (osm_id);
