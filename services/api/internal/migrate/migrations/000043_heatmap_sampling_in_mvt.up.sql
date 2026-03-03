-- Update heatmap_lines_mvt to support sample_interval_m query parameter.
-- When sample_interval_m is provided, ST_Simplify is applied to the track geometry
-- before rendering, reducing detail proportionally to the interval value.
-- This makes the sampling-interval setting in SettingsPage visually effective:
-- larger values → fewer line vertices → coarser appearance (faster tiles).
--
-- Conversion: degrees ≈ meters / 111320 (at equator, good enough for simplification)
-- sample_interval_m = 10  → tolerance ≈ 0.0000898° (high detail)
-- sample_interval_m = 100 → tolerance ≈ 0.000898°  (low detail)

CREATE OR REPLACE FUNCTION heatmap_lines_mvt(z INTEGER, x INTEGER, y INTEGER, query_params JSON DEFAULT '{}'::JSON)
RETURNS BYTEA LANGUAGE plpgsql STABLE PARALLEL SAFE AS $$
DECLARE
    bounds              GEOMETRY;
    sport_filter        TEXT;
    sub_type_filter     TEXT;
    date_from           DATE;
    date_to             DATE;
    threshold_m         FLOAT;
    sample_interval_m   FLOAT;
    simplify_tolerance  FLOAT;
    mvt                 BYTEA;
BEGIN
    bounds          := ST_TileEnvelope(z, x, y);
    sport_filter    := NULLIF(TRIM(query_params->>'sport_type'), '');
    sub_type_filter := NULLIF(TRIM(query_params->>'sub_type'), '');
    date_from       := NULLIF(TRIM(query_params->>'date_from'), '')::DATE;
    date_to         := NULLIF(TRIM(query_params->>'date_to'), '')::DATE;
    -- teleport_threshold_m: 0 oder fehlt => kein Filter (sehr grosser Wert)
    threshold_m     := COALESCE(NULLIF(TRIM(query_params->>'teleport_threshold_m'), '')::FLOAT, 0);
    IF threshold_m <= 0 THEN threshold_m := 1e9; END IF;
    -- sample_interval_m: steuert ST_Simplify-Toleranz; 0/fehlt => keine Vereinfachung
    -- (Standardwert 20m aus heatmap_sample_interval_m in users-Tabelle)
    sample_interval_m  := COALESCE(NULLIF(TRIM(query_params->>'sample_interval_m'), '')::FLOAT, 0);
    -- Toleranz in Grad: interval_m / 111320; bei 0 kein Simplify (preserve_collapsed=true ist harmlos)
    simplify_tolerance := CASE WHEN sample_interval_m > 0 THEN sample_interval_m / 111320.0 ELSE 0 END;

    SELECT ST_AsMVT(tile_data.*, 'heatmap_lines', 4096, 'geom') INTO mvt
    FROM (
        SELECT hl.id, hl.sport_type, hl.sub_type, hl.activity_date,
               ST_AsMVTGeom(simplified.geom_3857, bounds, 4096, 256, true) AS geom
        FROM heatmap_lines hl
        CROSS JOIN LATERAL (
            SELECT CASE
                WHEN simplify_tolerance > 0 THEN
                    -- Simplify each segment group after teleport filtering
                    ST_Simplify(
                        ST_Collect(seg.geom ORDER BY seg.path[1]),
                        simplify_tolerance,
                        true  -- preserve_collapsed: behalte sehr kurze Linien
                    )
                ELSE
                    ST_Collect(seg.geom ORDER BY seg.path[1])
            END AS geom_3857
            FROM (
                SELECT (ds).path AS path,
                       ST_Transform((ds).geom, 3857) AS geom,
                       ST_Length((ds).geom::geography) AS len_m
                FROM (SELECT ST_DumpSegments(hl.geom) AS ds) raw
            ) seg
            WHERE seg.len_m <= threshold_m
        ) simplified
        WHERE hl.geom && ST_Transform(bounds, 4326)
          AND simplified.geom_3857 IS NOT NULL
          AND (sport_filter    IS NULL OR hl.sport_type = ANY(string_to_array(sport_filter, ',')))
          AND (sub_type_filter IS NULL OR hl.sub_type   = ANY(string_to_array(sub_type_filter, ',')))
          AND (date_from       IS NULL OR hl.activity_date >= date_from)
          AND (date_to         IS NULL OR hl.activity_date <= date_to)
    ) tile_data
    WHERE tile_data.geom IS NOT NULL;

    RETURN COALESCE(mvt, ''::BYTEA);
END;
$$;
