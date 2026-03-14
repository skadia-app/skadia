-- 000157_routing_enrichment_columns.up.sql

-- Spatial Enrichment Spalten auf osm_ways
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS forest_pct SMALLINT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS noise_score SMALLINT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS traffic_class SMALLINT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS protected_area SMALLINT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS landuse_class SMALLINT;

-- Route-Relation Membership
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS is_cycleroute BOOLEAN DEFAULT false;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS is_hikeroute BOOLEAN DEFAULT false;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS cycleroute_network TEXT;
ALTER TABLE osm_ways ADD COLUMN IF NOT EXISTS hikeroute_network TEXT;

-- Junction-Table fuer Route-Membership-Details
CREATE TABLE IF NOT EXISTS osm_route_way_members (
    route_osm_id BIGINT NOT NULL,
    way_osm_id BIGINT NOT NULL,
    PRIMARY KEY (route_osm_id, way_osm_id)
);
CREATE INDEX IF NOT EXISTS idx_osm_route_way_members_way ON osm_route_way_members(way_osm_id);

-- Indexes fuer Enrichment-Abfragen
CREATE INDEX IF NOT EXISTS idx_osm_ways_forest_pct ON osm_ways(forest_pct) WHERE forest_pct IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_osm_ways_traffic_class ON osm_ways(traffic_class) WHERE traffic_class IS NOT NULL;

-- dem.get_elevation() erweitern: GLO-30 als dritte Fallback-Stufe
CREATE OR REPLACE FUNCTION dem.get_elevation(point GEOMETRY(Point, 4326))
RETURNS DOUBLE PRECISION AS $$
DECLARE
    elev DOUBLE PRECISION;
BEGIN
    -- 1. LiDAR (hoechste Prioritaet)
    SELECT ST_Value(rast, point) INTO elev
    FROM dem.raster_tiles
    WHERE source LIKE 'lidar_%'
    AND ST_Intersects(rast, point)
    ORDER BY resolution_m ASC
    LIMIT 1;
    IF elev IS NOT NULL THEN RETURN elev; END IF;

    -- 2. EEA-10
    SELECT ST_Value(rast, point) INTO elev
    FROM dem.raster_tiles
    WHERE source = 'eea10'
    AND ST_Intersects(rast, point)
    LIMIT 1;
    IF elev IS NOT NULL THEN RETURN elev; END IF;

    -- 3. GLO-30 (niedrigste Aufloesung)
    SELECT ST_Value(rast, point) INTO elev
    FROM dem.raster_tiles
    WHERE source = 'glo30'
    AND ST_Intersects(rast, point)
    LIMIT 1;

    RETURN elev;
END;
$$ LANGUAGE plpgsql STABLE;
