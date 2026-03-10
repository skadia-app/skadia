-- 000130_dem_schema.down.sql
DROP FUNCTION IF EXISTS dem.get_elevation_profile;
DROP FUNCTION IF EXISTS dem.get_elevation;
DROP TABLE IF EXISTS dem.raster_tiles;
DROP SCHEMA IF EXISTS dem;
