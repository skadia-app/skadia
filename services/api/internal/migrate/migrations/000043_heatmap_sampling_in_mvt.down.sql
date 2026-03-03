-- Revert heatmap_lines_mvt to version from 000024 (without sample_interval_m support)
CREATE OR REPLACE FUNCTION heatmap_lines_mvt(z INTEGER, x INTEGER, y INTEGER, query_params JSON DEFAULT '{}'::JSON)
RETURNS BYTEA LANGUAGE plpgsql STABLE PARALLEL SAFE AS $$
DECLARE
    bounds              GEOMETRY;
    sport_filter        TEXT;
    sub_type_filter     TEXT;
    date_from           DATE;
    date_to             DATE;
    threshold_m         FLOAT;
    mvt                 BYTEA;
BEGIN
    bounds          := ST_TileEnvelope(z, x, y);
    sport_filter    := NULLIF(TRIM(query_params->>'sport_type'), '');
    sub_type_filter := NULLIF(TRIM(query_params->>'sub_type'), '');
    date_from       := NULLIF(TRIM(query_params->>'date_from'), '')::DATE;
    date_to         := NULLIF(TRIM(query_params->>'date_to'), '')::DATE;
    threshold_m     := COALESCE(NULLIF(TRIM(query_params->>'teleport_threshold_m'), '')::FLOAT, 0);
    IF threshold_m <= 0 THEN threshold_m := 1e9; END IF;

    SELECT ST_AsMVT(tile_data.*, 'heatmap_lines', 4096, 'geom') INTO mvt
    FROM (
        SELECT hl.id, hl.sport_type, hl.sub_type, hl.activity_date,
               ST_AsMVTGeom(cleaned.geom_3857, bounds, 4096, 256, true) AS geom
        FROM heatmap_lines hl
        CROSS JOIN LATERAL (
            SELECT ST_Collect(seg.geom ORDER BY seg.path[1]) AS geom_3857
            FROM (
                SELECT (ds).path AS path,
                       ST_Transform((ds).geom, 3857) AS geom,
                       ST_Length((ds).geom::geography) AS len_m
                FROM (SELECT ST_DumpSegments(hl.geom) AS ds) raw
            ) seg
            WHERE seg.len_m <= threshold_m
        ) cleaned
        WHERE hl.geom && ST_Transform(bounds, 4326)
          AND cleaned.geom_3857 IS NOT NULL
          AND (sport_filter    IS NULL OR hl.sport_type = ANY(string_to_array(sport_filter, ',')))
          AND (sub_type_filter IS NULL OR hl.sub_type   = ANY(string_to_array(sub_type_filter, ',')))
          AND (date_from       IS NULL OR hl.activity_date >= date_from)
          AND (date_to         IS NULL OR hl.activity_date <= date_to)
    ) tile_data
    WHERE tile_data.geom IS NOT NULL;

    RETURN COALESCE(mvt, ''::BYTEA);
END;
$$;
