-- Migration 000104: Add user_id filter to heatmap MVT functions
-- Critical privacy fix: without this, any authenticated user can see all users' GPS tracks

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
    bounds              := ST_TileEnvelope(z, x, y);
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
                    (ds).path                                    AS path,
                    ST_Transform((ds).geom, 3857)               AS geom,
                    ST_Length((ds).geom::geography)             AS len_m
                FROM (
                    SELECT ST_DumpSegments(hl.geom) AS ds
                ) raw
            ) seg
            WHERE seg.len_m <= COALESCE(teleport_threshold, 2000)
        ) cleaned
        WHERE hl.geom && ST_Transform(bounds, 4326)
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
