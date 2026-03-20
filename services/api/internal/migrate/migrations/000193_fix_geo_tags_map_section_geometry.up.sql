-- Migration 000193: Widen map_section column from LineString to generic Geometry
-- Root cause: ST_Intersection of buffer polygon with waterways can return
-- MULTILINESTRING (when a waterway crosses the buffer more than once).
-- The GEOMETRY(LineString, 4326) constraint rejected these inserts, causing
-- the entire water-enrichment batch (still_water + flowing_water) to fail.
ALTER TABLE activity_geo_tags
    ALTER COLUMN map_section TYPE GEOMETRY(Geometry, 4326)
        USING map_section::GEOMETRY(Geometry, 4326);
