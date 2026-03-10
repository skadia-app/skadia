-- 000135_osm_trails_indexes.down.sql
DROP INDEX IF EXISTS idx_osm_trails_osm_id;
COMMENT ON FUNCTION tobler_speed(double precision) IS NULL;
ALTER TABLE activities DROP CONSTRAINT IF EXISTS chk_distance_source;
