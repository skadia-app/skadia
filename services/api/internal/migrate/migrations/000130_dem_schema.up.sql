-- 000130_dem_schema.up.sql
CREATE EXTENSION IF NOT EXISTS postgis_raster;
CREATE SCHEMA IF NOT EXISTS dem;

CREATE TABLE dem.raster_tiles (
    rid         SERIAL PRIMARY KEY,
    source      TEXT NOT NULL,
    resolution_m DOUBLE PRECISION NOT NULL,
    rast        RASTER NOT NULL
);
CREATE INDEX idx_dem_raster_geom ON dem.raster_tiles USING GIST(ST_ConvexHull(rast));

-- Höhenabfrage mit LiDAR-Fallback
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

-- Höhenprofil für LineString
CREATE OR REPLACE FUNCTION dem.get_elevation_profile(
    line GEOMETRY(LineString, 4326),
    num_points INTEGER DEFAULT 100
)
RETURNS TABLE(fraction DOUBLE PRECISION, elevation DOUBLE PRECISION, point GEOMETRY) AS $$
BEGIN
    RETURN QUERY
    SELECT
        f,
        dem.get_elevation(ST_LineInterpolatePoint(line, f)),
        ST_LineInterpolatePoint(line, f)
    FROM generate_series(0.0, 1.0, 1.0 / GREATEST(num_points - 1, 1)) AS f;
END;
$$ LANGUAGE plpgsql STABLE;
