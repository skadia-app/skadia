-- 000164_source_region.down.sql
DROP TABLE IF EXISTS osm_way_regions;
DROP TABLE IF EXISTS osm_route_regions;
ALTER TABLE osm_ways DROP CONSTRAINT IF EXISTS osm_ways_osm_id_unique;
ALTER TABLE osm_routes DROP CONSTRAINT IF EXISTS osm_routes_osm_id_unique;

-- source_region Spalten entfernen
ALTER TABLE osm_pois DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_forests DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_boundaries DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_landuse DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_railways DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_waterways DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_waters DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_glaciers DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_wetlands DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_pistes DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_piste_areas DROP COLUMN IF EXISTS source_region;
ALTER TABLE osm_lifts DROP COLUMN IF EXISTS source_region;

-- osm_import_regions Spalten entfernen
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS replication_url;
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS last_sequence_number;
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS pbf_path;
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS update_interval;
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS update_weekday;
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS update_day;
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS update_hour;
ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS data_date;

-- enrichment Schema wiederherstellen (für Rollback-Kompatibilität)
CREATE SCHEMA IF NOT EXISTS enrichment;
CREATE TABLE IF NOT EXISTS enrichment.raster_tiles (
    rid SERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    rast RASTER NOT NULL,
    resolution_m REAL NOT NULL
);
