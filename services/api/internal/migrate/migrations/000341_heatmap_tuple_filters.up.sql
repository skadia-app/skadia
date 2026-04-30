CREATE OR REPLACE FUNCTION skadia_sport_tuple_filter_matches(
    sport_filter TEXT,
    row_sport_type TEXT,
    row_sub_type TEXT
)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT sport_filter IS NULL OR EXISTS (
        SELECT 1
        FROM unnest(string_to_array(sport_filter, ',')) raw_token
        CROSS JOIN LATERAL (
            SELECT
                NULLIF(TRIM(split_part(raw_token, ':', 1)), '') AS sport_type,
                CASE
                    WHEN position(':' in raw_token) > 0
                    THEN TRIM(substr(raw_token, position(':' in raw_token) + 1))
                    ELSE NULL
                END AS sub_type
        ) token
        WHERE token.sport_type = row_sport_type
          AND (token.sub_type IS NULL OR token.sub_type = COALESCE(row_sub_type, ''))
    );
$$;

DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND (sport_filter     IS NULL OR hl.sport_type = ANY(string_to_array(sport_filter,    '','')))',
        'AND skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000341 could not patch heatmap_lines_mvt sport_filter predicate';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND (sport_filter     IS NULL OR hp.sport_type = ANY(string_to_array(sport_filter,    '','')))',
        'AND skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000341 could not patch heatmap_points_mvt sport_filter predicate';
    END IF;
    EXECUTE patched;
END $$;
