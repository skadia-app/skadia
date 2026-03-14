-- 000159_osm_pois_mixed_geometry.down.sql
-- Revert to Point-only (drops any polygon rows — data loss!)
DELETE FROM osm_pois WHERE ST_GeometryType(geom) != 'ST_Point';
ALTER TABLE osm_pois
  ALTER COLUMN geom TYPE geometry(Point, 4326);
