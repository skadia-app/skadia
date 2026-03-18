-- 000181_fix_elevation_profile_pg18.down.sql
-- Revert to pre-PG18 version (without explicit cast).
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
