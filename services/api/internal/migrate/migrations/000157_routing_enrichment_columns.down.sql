-- 000157_routing_enrichment_columns.down.sql

-- Indexes werden automatisch mit den Spalten gedroppt (PostgreSQL-Default)
ALTER TABLE osm_ways DROP COLUMN IF EXISTS forest_pct;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS noise_score;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS traffic_class;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS protected_area;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS landuse_class;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS is_cycleroute;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS is_hikeroute;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS cycleroute_network;
ALTER TABLE osm_ways DROP COLUMN IF EXISTS hikeroute_network;
DROP TABLE IF EXISTS osm_route_way_members;

-- Restore original dem.get_elevation without GLO-30 fallback
CREATE OR REPLACE FUNCTION dem.get_elevation(point GEOMETRY(Point, 4326))
RETURNS DOUBLE PRECISION AS $$
DECLARE
    elev DOUBLE PRECISION;
BEGIN
    SELECT ST_Value(rast, point) INTO elev
    FROM dem.raster_tiles
    WHERE ST_Intersects(rast, point)
      AND source LIKE 'lidar_%'
    ORDER BY resolution_m ASC
    LIMIT 1;
    IF elev IS NOT NULL THEN RETURN elev; END IF;
    SELECT ST_Value(rast, point) INTO elev
    FROM dem.raster_tiles
    WHERE ST_Intersects(rast, point)
      AND source = 'eea10'
    LIMIT 1;
    RETURN elev;
END;
$$ LANGUAGE plpgsql STABLE;
