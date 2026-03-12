-- ============================================================
-- Phase 8c Schema Rewrite: Unified osm_pois + Extended osm_ways
-- ============================================================

-- 1. Drop old point tables (Daten werden neu importiert)
DROP TABLE IF EXISTS osm_peaks CASCADE;
DROP TABLE IF EXISTS osm_passes CASCADE;
DROP TABLE IF EXISTS osm_settlements CASCADE;
DROP TABLE IF EXISTS osm_amenities CASCADE;

-- 2. Drop Martin MVT functions that reference old tables
DROP FUNCTION IF EXISTS peaks_mvt(integer, integer, integer);
DROP FUNCTION IF EXISTS amenities_mvt(integer, integer, integer);

-- 3. Unified POI table
CREATE TABLE osm_pois (
    id              BIGSERIAL PRIMARY KEY,
    osm_id          BIGINT NOT NULL,
    name            TEXT,
    category        TEXT NOT NULL,
    type            TEXT NOT NULL,
    subtype         TEXT,
    ele             REAL,
    tags            JSONB,
    geom            GEOMETRY(Point, 4326) NOT NULL
);

-- Partielle GIST-Indexes pro Kategorie (schneller als ein großer Index)
CREATE INDEX idx_osm_pois_geom ON osm_pois USING GIST(geom);
CREATE INDEX idx_osm_pois_peak ON osm_pois USING GIST(geom) WHERE category = 'peak';
CREATE INDEX idx_osm_pois_pass ON osm_pois USING GIST(geom) WHERE category = 'pass';
CREATE INDEX idx_osm_pois_settlement ON osm_pois USING GIST(geom) WHERE category = 'settlement';
CREATE INDEX idx_osm_pois_gastronomy ON osm_pois USING GIST(geom) WHERE category = 'gastronomy';
CREATE INDEX idx_osm_pois_accommodation ON osm_pois USING GIST(geom) WHERE category = 'accommodation';
CREATE INDEX idx_osm_pois_shelter ON osm_pois USING GIST(geom) WHERE category = 'shelter';
CREATE INDEX idx_osm_pois_water_supply ON osm_pois USING GIST(geom) WHERE category = 'water_supply';
CREATE INDEX idx_osm_pois_transport ON osm_pois USING GIST(geom) WHERE category = 'transport';
CREATE INDEX idx_osm_pois_parking ON osm_pois USING GIST(geom) WHERE category = 'parking';
CREATE INDEX idx_osm_pois_viewpoint ON osm_pois USING GIST(geom) WHERE category = 'viewpoint';
CREATE INDEX idx_osm_pois_shop ON osm_pois USING GIST(geom) WHERE category = 'shop';
CREATE INDEX idx_osm_pois_barrier ON osm_pois USING GIST(geom) WHERE category = 'barrier';
CREATE INDEX idx_osm_pois_category ON osm_pois(category);
-- Abhängigkeit: pg_trgm Extension (vorhanden seit Migration 000131)
CREATE INDEX idx_osm_pois_name_trgm ON osm_pois USING GIN(name gin_trgm_ops) WHERE name IS NOT NULL;

-- 4. Rename osm_trails → osm_ways + rename associated indexes
ALTER TABLE osm_trails RENAME TO osm_ways;

-- Rename associated indexes
ALTER INDEX IF EXISTS idx_osm_trails_geom RENAME TO idx_osm_ways_geom;
ALTER INDEX IF EXISTS idx_osm_trails_source RENAME TO idx_osm_ways_source;
ALTER INDEX IF EXISTS idx_osm_trails_target RENAME TO idx_osm_ways_target;
ALTER INDEX IF EXISTS idx_osm_trails_highway RENAME TO idx_osm_ways_highway;
ALTER INDEX IF EXISTS idx_osm_trails_osm_id RENAME TO idx_osm_ways_osm_id;

-- Rename vertices table if it exists (created by pgr_createTopology, may not exist yet)
DO $$ BEGIN
    ALTER TABLE osm_trails_vertices_pgr RENAME TO osm_ways_vertices_pgr;
EXCEPTION WHEN undefined_table THEN NULL;
END $$;

-- Note: osm_trails_vertices_pgr does not exist (pgr_createTopology not yet run)
-- It will be created as osm_ways_vertices_pgr when topology is built after OSM import.

-- Add new columns to osm_ways
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS ref TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS smoothness TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS mtb_scale_uphill TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS via_ferrata_scale TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS trail_visibility TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS width TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS incline TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS lanes SMALLINT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS cycleway TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS cycleway_left TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS cycleway_right TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS cycleway_both TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS bicycle_road BOOLEAN DEFAULT FALSE;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS segregated BOOLEAN;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS sidewalk TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS maxspeed TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS motor_vehicle TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS access TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS foot TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS bicycle TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS horse TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS ski TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS oneway_bicycle BOOLEAN;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS ford BOOLEAN DEFAULT FALSE;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS winter_service BOOLEAN;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS network TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS operator TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS tags JSONB;

-- 5. New line tables
CREATE TABLE osm_pistes (
    id              BIGSERIAL PRIMARY KEY,
    osm_id          BIGINT NOT NULL,
    name            TEXT,
    piste_type      TEXT NOT NULL,
    difficulty      TEXT,
    grooming        TEXT,
    lit             BOOLEAN DEFAULT FALSE,
    tags            JSONB,
    geom            GEOMETRY(LineString, 4326) NOT NULL
);
CREATE INDEX idx_osm_pistes_geom ON osm_pistes USING GIST(geom);
CREATE INDEX idx_osm_pistes_type ON osm_pistes(piste_type);

-- 6. New area tables
CREATE TABLE osm_glaciers (
    id              BIGSERIAL PRIMARY KEY,
    osm_id          BIGINT NOT NULL,
    name            TEXT,
    geom            GEOMETRY(MultiPolygon, 4326) NOT NULL
);
CREATE INDEX idx_osm_glaciers_geom ON osm_glaciers USING GIST(geom);

CREATE TABLE osm_wetlands (
    id              BIGSERIAL PRIMARY KEY,
    osm_id          BIGINT NOT NULL,
    name            TEXT,
    wetland_type    TEXT,
    geom            GEOMETRY(MultiPolygon, 4326) NOT NULL
);
CREATE INDEX idx_osm_wetlands_geom ON osm_wetlands USING GIST(geom);

-- 7. Update heatmap_trail_matches: trail_id → way_id
DO $$ BEGIN
    ALTER TABLE heatmap_trail_matches
        RENAME CONSTRAINT heatmap_trail_matches_trail_id_fkey
        TO heatmap_trail_matches_way_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;
DO $$ BEGIN
    ALTER TABLE heatmap_trail_matches RENAME COLUMN trail_id TO way_id;
EXCEPTION WHEN undefined_column THEN NULL;
END $$;

-- 8. lz4 TOAST compression on geometry columns
-- osm_pois / osm_pistes / osm_glaciers / osm_wetlands (always exist, just created)
ALTER TABLE osm_pois ALTER COLUMN geom SET COMPRESSION lz4;
ALTER TABLE osm_pois ALTER COLUMN tags SET COMPRESSION lz4;
ALTER TABLE osm_ways ALTER COLUMN geom SET COMPRESSION lz4;
ALTER TABLE osm_ways ALTER COLUMN tags SET COMPRESSION lz4;
ALTER TABLE osm_pistes ALTER COLUMN geom SET COMPRESSION lz4;
ALTER TABLE osm_pistes ALTER COLUMN tags SET COMPRESSION lz4;
ALTER TABLE osm_glaciers ALTER COLUMN geom SET COMPRESSION lz4;
ALTER TABLE osm_wetlands ALTER COLUMN geom SET COMPRESSION lz4;

-- Conditional lz4 compression for pre-existing OSM tables
DO $$ BEGIN
    ALTER TABLE osm_forests ALTER COLUMN geom SET COMPRESSION lz4;
EXCEPTION WHEN undefined_table OR undefined_column THEN NULL;
END $$;
DO $$ BEGIN
    ALTER TABLE osm_waters ALTER COLUMN geom SET COMPRESSION lz4;
EXCEPTION WHEN undefined_table OR undefined_column THEN NULL;
END $$;
DO $$ BEGIN
    ALTER TABLE osm_waterways ALTER COLUMN geom SET COMPRESSION lz4;
EXCEPTION WHEN undefined_table OR undefined_column THEN NULL;
END $$;
DO $$ BEGIN
    ALTER TABLE osm_lifts ALTER COLUMN geom SET COMPRESSION lz4;
EXCEPTION WHEN undefined_table OR undefined_column THEN NULL;
END $$;
DO $$ BEGIN
    ALTER TABLE osm_boundaries ALTER COLUMN geom SET COMPRESSION lz4;
EXCEPTION WHEN undefined_table OR undefined_column THEN NULL;
END $$;

-- 9. Update pgRouting cost function to reference osm_ways_vertices_pgr
-- Uses dynamic SQL (EXECUTE) so the table name is resolved at call time,
-- not at function-creation time — safe even if the topology table doesn't exist yet.
CREATE OR REPLACE FUNCTION nearest_trail_node(point GEOMETRY(Point, 4326))
RETURNS BIGINT AS $$
DECLARE
    result BIGINT;
BEGIN
    EXECUTE 'SELECT id FROM osm_ways_vertices_pgr ORDER BY the_geom <-> $1 LIMIT 1'
        INTO result USING point;
    RETURN result;
END;
$$ LANGUAGE plpgsql STABLE;

-- Grant read access to new tables for skadia_readonly (SQL Agent)
GRANT SELECT ON osm_pois, osm_pistes, osm_glaciers, osm_wetlands TO skadia_readonly;
GRANT SELECT ON osm_ways TO skadia_readonly;
