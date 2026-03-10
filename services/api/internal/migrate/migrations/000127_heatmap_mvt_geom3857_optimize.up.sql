-- Migration 000127: Optimize heatmap_lines_mvt to use pre-transformed geom_3857 column
--
-- Change from 000125:
--   1. Bounding-Box-Filter: hl.geom && ST_Transform(bounds, 4326)
--                      --> hl.geom_3857 && bounds
--      bounds is EPSG:3857 (from ST_TileEnvelope), so we can directly hit the
--      GIST index on geom_3857 (created in 000090) without the ST_Transform overhead.
--
--   2. Segment dump: ST_Transform((ds).geom, 3857) on hl.geom (4326 geometry)
--               --> (ds).geom directly from ST_DumpSegments(hl.geom_3857)
--      geom_3857 is already Web Mercator, no per-segment transform needed.
--
--   3. Teleport filter: ST_Length((ds).geom::geography) in meters (accurate great-circle)
--                  --> ST_Length((ds).geom) on 3857 geometry
--      Web Mercator (EPSG:3857) uses meters as the unit. ST_Length on 3857 gives metric
--      distances — sufficient precision for the teleport heuristic (default 2000 m).
--      Note: 3857 distances are slightly distorted at high latitudes but are accurate
--      enough for segment filtering where we only reject obvious teleports.
--
-- All other filters (user_id, sport_type, sub_type, date, tags, gear) are unchanged.
-- heatmap_points_mvt is also re-created verbatim (no changes, required to keep function
-- consistent with heatmap_lines_mvt in the same migration).

CREATE OR REPLACE FUNCTION heatmap_lines_mvt(
    z            INTEGER,
    x            INTEGER,
    y            INTEGER,
    query_params JSON DEFAULT '{}'::JSON
)
RETURNS BYTEA
LANGUAGE plpgsql
STABLE
PARALLEL SAFE
AS $$
DECLARE
    bounds              GEOMETRY;
    user_filter         UUID;
    sport_filter        TEXT;
    sub_type_filter     TEXT;
    date_from           DATE;
    date_to             DATE;
    teleport_threshold  FLOAT8;
    sample_interval     FLOAT8;
    include_tags_arr    UUID[];
    exclude_tags_arr    UUID[];
    include_gear_arr    UUID[];
    exclude_gear_arr    UUID[];
    mvt                 BYTEA;
BEGIN
    bounds              := ST_TileEnvelope(z, x, y);  -- result is EPSG:3857
    user_filter         := NULLIF(TRIM(query_params->>'user_id'), '')::UUID;
    sport_filter        := NULLIF(TRIM(query_params->>'sport_type'), '');
    sub_type_filter     := NULLIF(TRIM(query_params->>'sub_type'), '');
    date_from           := NULLIF(TRIM(query_params->>'date_from'), '')::DATE;
    date_to             := NULLIF(TRIM(query_params->>'date_to'), '')::DATE;
    teleport_threshold  := NULLIF(TRIM(query_params->>'teleport_threshold_m'), '')::FLOAT8;
    sample_interval     := NULLIF(TRIM(query_params->>'sample_interval_m'), '')::FLOAT8;

    -- Tag/Gear filter arrays (comma-separated UUIDs)
    SELECT array_agg(v::UUID) INTO include_tags_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'include_tags'), ''), ',')) v;
    SELECT array_agg(v::UUID) INTO exclude_tags_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'exclude_tags'), ''), ',')) v;
    SELECT array_agg(v::UUID) INTO include_gear_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'include_gear'), ''), ',')) v;
    SELECT array_agg(v::UUID) INTO exclude_gear_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'exclude_gear'), ''), ',')) v;

    SELECT ST_AsMVT(tile_data.*, 'heatmap_lines', 4096, 'geom')
    INTO mvt
    FROM (
        SELECT
            hl.id,
            hl.sport_type,
            hl.sub_type,
            hl.activity_date,
            ST_AsMVTGeom(
                cleaned.geom_3857,
                bounds,
                4096,
                256,
                true
            ) AS geom
        FROM heatmap_lines hl
        CROSS JOIN LATERAL (
            SELECT ST_Collect(seg.geom ORDER BY seg.path[1]) AS geom_3857
            FROM (
                SELECT
                    (ds).path                  AS path,
                    (ds).geom                  AS geom,        -- already 3857
                    ST_Length((ds).geom)       AS len_m        -- 3857 units = meters
                FROM (
                    SELECT ST_DumpSegments(hl.geom_3857) AS ds  -- use pre-transformed column
                ) raw
            ) seg
            WHERE seg.len_m <= COALESCE(teleport_threshold, 2000)
        ) cleaned
        -- Bounding box filter: geom_3857 && bounds — both EPSG:3857, hits GIST index
        WHERE hl.geom_3857 && bounds
          AND cleaned.geom_3857 IS NOT NULL
          AND (user_filter      IS NULL OR hl.user_id = user_filter)
          AND (sport_filter     IS NULL OR hl.sport_type = ANY(string_to_array(sport_filter,    ',')))
          AND (sub_type_filter  IS NULL OR hl.sub_type   = ANY(string_to_array(sub_type_filter, ',')))
          AND (date_from        IS NULL OR hl.activity_date >= date_from)
          AND (date_to          IS NULL OR hl.activity_date <= date_to)
          -- Tag filter
          AND (include_tags_arr IS NULL OR hl.activity_id IN (
                SELECT activity_id FROM activity_tags WHERE tag_id = ANY(include_tags_arr)
              ))
          AND (exclude_tags_arr IS NULL OR hl.activity_id NOT IN (
                SELECT activity_id FROM activity_tags WHERE tag_id = ANY(exclude_tags_arr)
              ))
          -- Gear filter
          AND (include_gear_arr IS NULL OR hl.activity_id IN (
                SELECT id FROM activities WHERE gear_id = ANY(include_gear_arr)
              ))
          AND (exclude_gear_arr IS NULL OR hl.activity_id NOT IN (
                SELECT id FROM activities WHERE gear_id = ANY(exclude_gear_arr)
              ))
    ) tile_data
    WHERE tile_data.geom IS NOT NULL;

    RETURN COALESCE(mvt, ''::BYTEA);
END;
$$;

-- heatmap_points_mvt: re-created verbatim from 000125 (no changes in this migration)
CREATE OR REPLACE FUNCTION heatmap_points_mvt(
    z            INTEGER,
    x            INTEGER,
    y            INTEGER,
    query_params JSON DEFAULT '{}'::JSON
)
RETURNS BYTEA
LANGUAGE plpgsql
STABLE
PARALLEL SAFE
AS $$
DECLARE
    bounds              GEOMETRY;
    user_filter         UUID;
    sport_filter        TEXT;
    sub_type_filter     TEXT;
    date_from           DATE;
    date_to             DATE;
    include_tags_arr    UUID[];
    exclude_tags_arr    UUID[];
    include_gear_arr    UUID[];
    exclude_gear_arr    UUID[];
    mvt                 BYTEA;
BEGIN
    bounds          := ST_TileEnvelope(z, x, y);
    user_filter     := NULLIF(TRIM(query_params->>'user_id'), '')::UUID;
    sport_filter    := NULLIF(TRIM(query_params->>'sport_type'), '');
    sub_type_filter := NULLIF(TRIM(query_params->>'sub_type'), '');
    date_from       := NULLIF(TRIM(query_params->>'date_from'), '')::DATE;
    date_to         := NULLIF(TRIM(query_params->>'date_to'), '')::DATE;

    SELECT array_agg(v::UUID) INTO include_tags_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'include_tags'), ''), ',')) v;
    SELECT array_agg(v::UUID) INTO exclude_tags_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'exclude_tags'), ''), ',')) v;
    SELECT array_agg(v::UUID) INTO include_gear_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'include_gear'), ''), ',')) v;
    SELECT array_agg(v::UUID) INTO exclude_gear_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'exclude_gear'), ''), ',')) v;

    SELECT ST_AsMVT(tile_data.*, 'heatmap_points', 4096, 'geom')
    INTO mvt
    FROM (
        SELECT
            hp.id,
            hp.sport_type,
            hp.sub_type,
            hp.activity_date,
            ST_AsMVTGeom(
                ST_Transform(hp.geom, 3857),
                bounds,
                4096,
                256,
                true
            ) AS geom
        FROM heatmap_points hp
        WHERE hp.geom && ST_Transform(bounds, 4326)
          AND (user_filter      IS NULL OR hp.user_id = user_filter)
          AND (sport_filter     IS NULL OR hp.sport_type = ANY(string_to_array(sport_filter,    ',')))
          AND (sub_type_filter  IS NULL OR hp.sub_type   = ANY(string_to_array(sub_type_filter, ',')))
          AND (date_from        IS NULL OR hp.activity_date >= date_from)
          AND (date_to          IS NULL OR hp.activity_date <= date_to)
          AND (include_tags_arr IS NULL OR hp.activity_id IN (
                SELECT activity_id FROM activity_tags WHERE tag_id = ANY(include_tags_arr)
              ))
          AND (exclude_tags_arr IS NULL OR hp.activity_id NOT IN (
                SELECT activity_id FROM activity_tags WHERE tag_id = ANY(exclude_tags_arr)
              ))
          AND (include_gear_arr IS NULL OR hp.activity_id IN (
                SELECT id FROM activities WHERE gear_id = ANY(include_gear_arr)
              ))
          AND (exclude_gear_arr IS NULL OR hp.activity_id NOT IN (
                SELECT id FROM activities WHERE gear_id = ANY(exclude_gear_arr)
              ))
    ) tile_data
    WHERE tile_data.geom IS NOT NULL;

    RETURN COALESCE(mvt, ''::BYTEA);
END;
$$;
