-- Phase 4a: OSM-Tabellen (Vorlage/Fallback fuer osm2pgsql Flex)

CREATE TABLE IF NOT EXISTS osm_boundaries (
    osm_id BIGINT PRIMARY KEY,
    name TEXT,
    admin_level INTEGER,
    country_code TEXT,
    geom GEOMETRY(MultiPolygon, 4326)
);
CREATE INDEX IF NOT EXISTS idx_osm_boundaries_geom ON osm_boundaries USING GIST (geom);
CREATE INDEX IF NOT EXISTS idx_osm_boundaries_admin ON osm_boundaries (admin_level);

CREATE TABLE IF NOT EXISTS osm_peaks (
    osm_id BIGINT PRIMARY KEY,
    name TEXT,
    ele DOUBLE PRECISION,
    type TEXT,
    geom GEOMETRY(Point, 4326)
);
CREATE INDEX IF NOT EXISTS idx_osm_peaks_geom ON osm_peaks USING GIST (geom);

CREATE TABLE IF NOT EXISTS osm_forests (
    osm_id BIGINT PRIMARY KEY,
    name TEXT,
    geom GEOMETRY(MultiPolygon, 4326)
);
CREATE INDEX IF NOT EXISTS idx_osm_forests_geom ON osm_forests USING GIST (geom);

CREATE TABLE IF NOT EXISTS osm_waters (
    osm_id BIGINT PRIMARY KEY,
    name TEXT,
    geom GEOMETRY(MultiPolygon, 4326)
);
CREATE INDEX IF NOT EXISTS idx_osm_waters_geom ON osm_waters USING GIST (geom);

CREATE TABLE IF NOT EXISTS osm_waterways (
    osm_id BIGINT PRIMARY KEY,
    name TEXT,
    waterway TEXT,
    geom GEOMETRY(LineString, 4326)
);
CREATE INDEX IF NOT EXISTS idx_osm_waterways_geom ON osm_waterways USING GIST (geom);

CREATE TABLE IF NOT EXISTS osm_settlements (
    osm_id BIGINT PRIMARY KEY,
    name TEXT,
    place TEXT,
    population INTEGER,
    geom GEOMETRY(Point, 4326)
);
CREATE INDEX IF NOT EXISTS idx_osm_settlements_geom ON osm_settlements USING GIST (geom);

CREATE TABLE IF NOT EXISTS osm_amenities (
    osm_id BIGINT PRIMARY KEY,
    name TEXT,
    type TEXT,
    subtype TEXT,
    geom GEOMETRY(Point, 4326)
);
CREATE INDEX IF NOT EXISTS idx_osm_amenities_geom ON osm_amenities USING GIST (geom);
