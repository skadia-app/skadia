-- 000076_amenities_mvt.up.sql
-- Martin Vector Tile Function für besuchte gastronomische Einrichtungen.
-- Joined osm_amenities mit activity_stops per poi_osm_id.

CREATE OR REPLACE FUNCTION amenities_mvt(
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
    SELECT ST_AsMVT(tile, 'amenities', 4096, 'geom')
    FROM (
      SELECT
        a.osm_id,
        a.name,
        a.subtype AS poi_type,
        COUNT(DISTINCT s.id) AS visit_count,
        ST_AsMVTGeom(ST_Transform(a.geom, 3857), bounds, 4096, 64, true) AS geom
      FROM osm_amenities a
      JOIN activity_stops s ON s.poi_osm_id = a.osm_id
      JOIN activities act ON act.id = s.activity_id
      WHERE ST_Transform(a.geom, 3857) && bounds
        AND a.subtype IN (
          'restaurant', 'cafe', 'bar', 'pub', 'fast_food', 'food_court',
          'ice_cream', 'biergarten', 'alpine_hut', 'guest_house', 'hostel',
          'hotel', 'kiosk', 'bakery', 'butcher'
        )
        AND (user_id IS NULL OR act.created_by = user_id)
      GROUP BY a.osm_id, a.name, a.subtype, a.geom
    ) tile
    WHERE tile.geom IS NOT NULL
  );
END;
$$ LANGUAGE plpgsql STABLE PARALLEL SAFE;
