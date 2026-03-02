-- Heatmap MVT Functions fuer Martin Tile Server (Function Sources)
-- Ersetzen die Table Sources, um echtes SQL-Filtering zu ermoeglichen.
-- Unterstuetzt: sport_type (kommagetrennt), date_from/date_to, sub_type.
-- Filtert ausserdem Teleportationslinien (Segmentspruenge > 2000m) heraus.

-- ============================================================
-- 1. heatmap_lines_mvt
-- ============================================================
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
    bounds          GEOMETRY;   -- 3857 tile envelope
    sport_filter    TEXT;
    sub_type_filter TEXT;
    date_from       DATE;
    date_to         DATE;
    mvt             BYTEA;
BEGIN
    bounds := ST_TileEnvelope(z, x, y);  -- EPSG:3857

    sport_filter    := NULLIF(TRIM(query_params->>'sport_type'), '');
    sub_type_filter := NULLIF(TRIM(query_params->>'sub_type'), '');
    date_from       := NULLIF(TRIM(query_params->>'date_from'), '')::DATE;
    date_to         := NULLIF(TRIM(query_params->>'date_to'), '')::DATE;

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
        -- Schritt 1: Segmente validieren und Teleportationen herausfiltern
        CROSS JOIN LATERAL (
            SELECT ST_Collect(seg.geom ORDER BY seg.path[1]) AS geom_3857
            FROM (
                -- Jeden Abschnitt der LineString einzeln pruefen
                SELECT
                    (ds).path                                    AS path,
                    ST_Transform((ds).geom, 3857)               AS geom,
                    ST_Length((ds).geom::geography)             AS len_m
                FROM (
                    SELECT ST_DumpSegments(hl.geom) AS ds
                ) raw
            ) seg
            WHERE seg.len_m <= 2000
        ) cleaned
        WHERE hl.geom && ST_Transform(bounds, 4326)
          AND cleaned.geom_3857 IS NOT NULL
          AND (sport_filter    IS NULL OR hl.sport_type = ANY(string_to_array(sport_filter,    ',')))
          AND (sub_type_filter IS NULL OR hl.sub_type   = ANY(string_to_array(sub_type_filter, ',')))
          AND (date_from       IS NULL OR hl.activity_date >= date_from)
          AND (date_to         IS NULL OR hl.activity_date <= date_to)
    ) tile_data
    WHERE tile_data.geom IS NOT NULL;

    RETURN COALESCE(mvt, ''::BYTEA);
END;
$$;

-- ============================================================
-- 2. heatmap_points_mvt
-- ============================================================
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
    bounds          GEOMETRY;   -- 3857 tile envelope
    sport_filter    TEXT;
    sub_type_filter TEXT;
    date_from       DATE;
    date_to         DATE;
    mvt             BYTEA;
BEGIN
    bounds := ST_TileEnvelope(z, x, y);  -- EPSG:3857

    sport_filter    := NULLIF(TRIM(query_params->>'sport_type'), '');
    sub_type_filter := NULLIF(TRIM(query_params->>'sub_type'), '');
    date_from       := NULLIF(TRIM(query_params->>'date_from'), '')::DATE;
    date_to         := NULLIF(TRIM(query_params->>'date_to'), '')::DATE;

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
          AND (sport_filter    IS NULL OR hp.sport_type = ANY(string_to_array(sport_filter,    ',')))
          AND (sub_type_filter IS NULL OR hp.sub_type   = ANY(string_to_array(sub_type_filter, ',')))
          AND (date_from       IS NULL OR hp.activity_date >= date_from)
          AND (date_to         IS NULL OR hp.activity_date <= date_to)
    ) tile_data
    WHERE tile_data.geom IS NOT NULL;

    RETURN COALESCE(mvt, ''::BYTEA);
END;
$$;
