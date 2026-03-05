-- 000069_mvt_functions.up.sql
-- Martin Vector Tile Functions für Gipfel, Gewässer und Wälder.
-- Verwendet ST_TileEnvelope (EPSG:3857) konsistent mit den Heatmap-Funktionen.

-- Gipfel: gibt MVT mit visited-Flag zurück
CREATE OR REPLACE FUNCTION peaks_mvt(
    z INTEGER, x INTEGER, y INTEGER,
    query_params JSON DEFAULT '{}'
)
RETURNS bytea AS $$
DECLARE
  bounds  geometry;
  user_id TEXT := query_params->>'user_id';
BEGIN
  bounds := ST_TileEnvelope(z, x, y);  -- EPSG:3857
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
          WHEN user_id IS NOT NULL AND EXISTS (
            SELECT 1 FROM activity_geo_tags g
            WHERE g.category IN ('summit','pass')
              AND g.osm_id = p.osm_id
              AND g.activity_id IN (
                SELECT id FROM activities WHERE created_by = user_id
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

-- Gewässer: Seen (Polygon-Outlines) + Flüsse (LineStrings)
CREATE OR REPLACE FUNCTION waters_mvt(
    z INTEGER, x INTEGER, y INTEGER,
    query_params JSON DEFAULT '{}'
)
RETURNS bytea AS $$
DECLARE bounds geometry;
BEGIN
  bounds := ST_TileEnvelope(z, x, y);
  RETURN (
    SELECT ST_AsMVT(tile, 'waters', 4096, 'geom')
    FROM (
      SELECT
        w.osm_id,
        w.name,
        'water' AS layer_type,
        ST_AsMVTGeom(
          ST_Boundary(ST_Transform(w.geom, 3857)),
          bounds, 4096, 64, true
        ) AS geom
      FROM osm_waters w
      WHERE ST_Transform(w.geom, 3857) && bounds
      UNION ALL
      SELECT
        ww.osm_id,
        ww.name,
        'waterway' AS layer_type,
        ST_AsMVTGeom(ST_Transform(ww.geom, 3857), bounds, 4096, 64, true) AS geom
      FROM osm_waterways ww
      WHERE ST_Transform(ww.geom, 3857) && bounds
    ) tile
    WHERE tile.geom IS NOT NULL
  );
END;
$$ LANGUAGE plpgsql STABLE PARALLEL SAFE;

-- Wälder: Polygon-Outlines mit visited-Flag
CREATE OR REPLACE FUNCTION forests_mvt(
    z INTEGER, x INTEGER, y INTEGER,
    query_params JSON DEFAULT '{}'
)
RETURNS bytea AS $$
DECLARE
  bounds  geometry;
  user_id TEXT := query_params->>'user_id';
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
          WHEN user_id IS NOT NULL AND EXISTS (
            SELECT 1 FROM activity_tracks t
            JOIN activities a ON a.id = t.activity_id
            WHERE a.created_by = user_id
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
