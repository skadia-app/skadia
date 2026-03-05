-- 000072_fix_mvt_user_id.up.sql
-- Korrigiert peaks_mvt und forests_mvt: activities.created_by → activities.user_id

CREATE OR REPLACE FUNCTION peaks_mvt(
    z INTEGER, x INTEGER, y INTEGER,
    query_params JSON DEFAULT '{}'
)
RETURNS bytea AS $$
DECLARE
  bounds  geometry;
  uid     TEXT := NULLIF(TRIM(query_params->>'user_id'), '');
BEGIN
  bounds := ST_TileEnvelope(z, x, y);
  RETURN (
    SELECT ST_AsMVT(tile, 'peaks', 4096, 'geom')
    FROM (
      SELECT
        p.osm_id,
        p.name,
        p.ele,
        p.type,
        ST_AsMVTGeom(ST_Transform(p.geom, 3857), bounds, 4096, 64, true) AS geom,
        CASE
          WHEN uid IS NOT NULL AND EXISTS (
            SELECT 1 FROM activity_geo_tags g
            WHERE g.category IN ('summit','pass')
              AND g.osm_id = p.osm_id
              AND g.activity_id IN (
                SELECT id FROM activities WHERE user_id = uid::uuid
              )
          ) THEN true
          ELSE false
        END AS visited
      FROM osm_peaks p
      WHERE ST_Transform(p.geom, 3857) && bounds
        AND p.name IS NOT NULL
    ) tile
    WHERE tile.geom IS NOT NULL
  );
END;
$$ LANGUAGE plpgsql STABLE PARALLEL SAFE;

CREATE OR REPLACE FUNCTION forests_mvt(
    z INTEGER, x INTEGER, y INTEGER,
    query_params JSON DEFAULT '{}'
)
RETURNS bytea AS $$
DECLARE
  bounds  geometry;
  uid     TEXT := NULLIF(TRIM(query_params->>'user_id'), '');
BEGIN
  bounds := ST_TileEnvelope(z, x, y);
  RETURN (
    SELECT ST_AsMVT(tile, 'forests', 4096, 'geom')
    FROM (
      SELECT
        f.osm_id,
        f.name,
        ST_AsMVTGeom(
          ST_Boundary(ST_Transform(f.geom, 3857)),
          bounds, 4096, 64, true
        ) AS geom,
        CASE
          WHEN uid IS NOT NULL AND EXISTS (
            SELECT 1 FROM activity_tracks t
            JOIN activities a ON a.id = t.activity_id
            WHERE a.user_id = uid::uuid
              AND ST_Intersects(f.geom, ST_Transform(t.track_full, 4326))
          ) THEN true
          ELSE false
        END AS visited
      FROM osm_forests f
      WHERE ST_Transform(f.geom, 3857) && bounds
    ) tile
    WHERE tile.geom IS NOT NULL
  );
END;
$$ LANGUAGE plpgsql STABLE PARALLEL SAFE;
