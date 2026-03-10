-- 000131_routing_cost_functions.up.sql

-- pg_trgm für POI-Fuzzy-Suche
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Tobler Hiking Function (Geschwindigkeit in km/h basierend auf Steigung)
CREATE OR REPLACE FUNCTION tobler_speed(slope_pct DOUBLE PRECISION)
RETURNS DOUBLE PRECISION AS $$
BEGIN
    RETURN 6.0 * EXP(-3.5 * ABS(slope_pct + 0.05));
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- SAC-Scale Kostenfaktor
CREATE OR REPLACE FUNCTION sac_cost_factor(sac TEXT)
RETURNS DOUBLE PRECISION AS $$
BEGIN
    RETURN CASE sac
        WHEN 'hiking'                    THEN 1.0
        WHEN 'mountain_hiking'           THEN 1.3
        WHEN 'demanding_mountain_hiking' THEN 1.8
        WHEN 'alpine_hiking'             THEN 2.5
        WHEN 'demanding_alpine_hiking'   THEN 3.5
        WHEN 'difficult_alpine_hiking'   THEN 5.0
        ELSE 1.0
    END;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Surface Kostenfaktor
CREATE OR REPLACE FUNCTION surface_cost_factor(surf TEXT)
RETURNS DOUBLE PRECISION AS $$
BEGIN
    RETURN CASE surf
        WHEN 'asphalt'        THEN 0.9
        WHEN 'concrete'       THEN 0.9
        WHEN 'paving_stones'  THEN 1.0
        WHEN 'compacted'      THEN 1.0
        WHEN 'fine_gravel'    THEN 1.1
        WHEN 'gravel'         THEN 1.2
        WHEN 'ground'         THEN 1.3
        WHEN 'dirt'           THEN 1.3
        WHEN 'grass'          THEN 1.5
        WHEN 'mud'            THEN 2.0
        WHEN 'sand'           THEN 2.5
        WHEN 'rock'           THEN 1.8
        WHEN 'scree'          THEN 2.5
        ELSE 1.2
    END;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Nearest trail node (für pgRouting Routing-Start/Ziel)
-- Uses plpgsql + EXECUTE to defer table resolution: osm_trails_vertices_pgr
-- is created by pgr_createTopology and does not exist at migration time.
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

-- POI-Fuzzy-Suche Indexes
CREATE INDEX IF NOT EXISTS idx_osm_peaks_name_trgm ON osm_peaks USING GIN(name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_osm_amenities_name_trgm ON osm_amenities USING GIN(name gin_trgm_ops);
