-- 000181_fix_elevation_profile_pg18.up.sql
-- Fix: PostgreSQL 18 returns numeric (not double precision) from
-- generate_series(0.0, 1.0, step). The RETURNS TABLE expects double precision,
-- causing "structure of query does not match function result type".
-- Fix: cast f::double precision explicitly.

CREATE OR REPLACE FUNCTION dem.get_elevation_profile(
    line GEOMETRY(LineString, 4326),
    num_points INTEGER DEFAULT 100
)
RETURNS TABLE(fraction DOUBLE PRECISION, elevation DOUBLE PRECISION, point GEOMETRY) AS $$
BEGIN
    RETURN QUERY
    SELECT
        f::double precision,
        dem.get_elevation(ST_LineInterpolatePoint(line, f::double precision)),
        ST_LineInterpolatePoint(line, f::double precision)
    FROM generate_series(0.0, 1.0, 1.0 / GREATEST(num_points - 1, 1)) AS f;
END;
$$ LANGUAGE plpgsql STABLE;
