-- Revert: Recreate old tables, rename back, drop new tables
-- NOTE: Data will be lost — reimport required after rollback

-- Rename way_id back to trail_id in heatmap_trail_matches
DO $$ BEGIN
    ALTER TABLE heatmap_trail_matches RENAME COLUMN way_id TO trail_id;
EXCEPTION WHEN undefined_column THEN NULL;
END $$;
DO $$ BEGIN
    ALTER TABLE heatmap_trail_matches
        RENAME CONSTRAINT heatmap_trail_matches_way_id_fkey
        TO heatmap_trail_matches_trail_id_fkey;
EXCEPTION WHEN undefined_object THEN NULL;
END $$;

-- Rename indexes back before renaming table
ALTER INDEX IF EXISTS idx_osm_ways_geom RENAME TO idx_osm_trails_geom;
ALTER INDEX IF EXISTS idx_osm_ways_source RENAME TO idx_osm_trails_source;
ALTER INDEX IF EXISTS idx_osm_ways_target RENAME TO idx_osm_trails_target;
ALTER INDEX IF EXISTS idx_osm_ways_highway RENAME TO idx_osm_trails_highway;
ALTER INDEX IF EXISTS idx_osm_ways_osm_id RENAME TO idx_osm_trails_osm_id;

-- Drop added columns (keep original osm_trails columns)
ALTER TABLE osm_ways DROP COLUMN IF EXISTS ref;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS smoothness;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS mtb_scale_uphill;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS via_ferrata_scale;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS trail_visibility;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS width;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS incline;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS lanes;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS cycleway;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS cycleway_left;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS cycleway_right;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS cycleway_both;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS bicycle_road;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS segregated;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS sidewalk;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS maxspeed;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS motor_vehicle;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS access;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS foot;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS bicycle;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS horse;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS ski;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS oneway_bicycle;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS ford;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS winter_service;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS network;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS operator;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS tags;

-- Rename osm_ways back to osm_trails
ALTER TABLE osm_ways RENAME TO osm_trails;

-- Recreate old point tables (empty — data must be reimported)
CREATE TABLE osm_peaks (
    id      BIGSERIAL PRIMARY KEY,
    osm_id  BIGINT,
    name    TEXT,
    ele     REAL,
    type    TEXT,
    geom    GEOMETRY(Point, 4326)
);
CREATE TABLE osm_passes (
    id      BIGSERIAL PRIMARY KEY,
    osm_id  BIGINT,
    name    TEXT,
    ele     REAL,
    geom    GEOMETRY(Point, 4326)
);
CREATE TABLE osm_settlements (
    id          BIGSERIAL PRIMARY KEY,
    osm_id      BIGINT,
    name        TEXT,
    place       TEXT,
    population  INT,
    geom        GEOMETRY(Point, 4326)
);
CREATE TABLE osm_amenities (
    id      BIGSERIAL PRIMARY KEY,
    osm_id  BIGINT,
    name    TEXT,
    type    TEXT,
    subtype TEXT,
    geom    GEOMETRY(Point, 4326)
);

-- Restore nearest_trail_node to reference old table name
CREATE OR REPLACE FUNCTION nearest_trail_node(point GEOMETRY(Point, 4326))
RETURNS BIGINT AS $$
DECLARE
    result BIGINT;
BEGIN
    EXECUTE 'SELECT id FROM osm_trails_vertices_pgr ORDER BY the_geom <-> $1 LIMIT 1'
        INTO result USING point;
    RETURN result;
END;
$$ LANGUAGE plpgsql STABLE;

-- Drop new tables
DROP TABLE IF EXISTS osm_pois CASCADE;
DROP TABLE IF EXISTS osm_pistes CASCADE;
DROP TABLE IF EXISTS osm_glaciers CASCADE;
DROP TABLE IF EXISTS osm_wetlands CASCADE;
