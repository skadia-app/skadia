-- 000161_raster_enrichment_tables.up.sql

CREATE TABLE osm_landuse (
    id BIGINT GENERATED ALWAYS AS IDENTITY,
    osm_id BIGINT NOT NULL,
    name TEXT,
    landuse TEXT NOT NULL,
    geom GEOMETRY(Geometry, 4326) NOT NULL
);
CREATE INDEX idx_osm_landuse_geom ON osm_landuse USING GIST (geom);
CREATE INDEX idx_osm_landuse_type ON osm_landuse(landuse);

CREATE TABLE osm_railways (
    id BIGINT GENERATED ALWAYS AS IDENTITY,
    osm_id BIGINT NOT NULL,
    name TEXT,
    railway TEXT NOT NULL,
    geom GEOMETRY(LineString, 4326) NOT NULL
);
CREATE INDEX idx_osm_railways_geom ON osm_railways USING GIST (geom);

CREATE SCHEMA IF NOT EXISTS enrichment;

CREATE TABLE enrichment.raster_tiles (
    rid SERIAL PRIMARY KEY,
    source TEXT NOT NULL,
    rast RASTER NOT NULL,
    resolution_m REAL NOT NULL
);
CREATE INDEX idx_enrichment_raster_geom
    ON enrichment.raster_tiles USING GIST (ST_ConvexHull(rast));
CREATE INDEX idx_enrichment_raster_source
    ON enrichment.raster_tiles(source);

CREATE TABLE osm_piste_areas (
    id BIGINT GENERATED ALWAYS AS IDENTITY,
    osm_id BIGINT NOT NULL,
    name TEXT,
    piste_type TEXT NOT NULL,
    difficulty TEXT,
    grooming TEXT,
    lit BOOLEAN DEFAULT false,
    tags JSONB,
    geom GEOMETRY(Geometry, 4326) NOT NULL
);
CREATE INDEX idx_osm_piste_areas_geom ON osm_piste_areas USING GIST (geom);

-- GRANTs NACH allen CREATE TABLEs
GRANT USAGE ON SCHEMA enrichment TO skadia_readonly;
GRANT SELECT ON enrichment.raster_tiles TO skadia_readonly;
GRANT SELECT ON osm_landuse TO skadia_readonly;
GRANT SELECT ON osm_railways TO skadia_readonly;
GRANT SELECT ON osm_piste_areas TO skadia_readonly;
