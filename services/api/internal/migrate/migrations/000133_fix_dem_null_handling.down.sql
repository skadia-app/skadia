-- 000133_fix_dem_null_handling.down.sql
-- Restore original functions from 000130 (backwards-compatible, no-op in practice).

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
