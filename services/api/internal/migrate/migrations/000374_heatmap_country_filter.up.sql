CREATE OR REPLACE FUNCTION skadia_heatmap_country_filter_matches(
    query_params JSON,
    activity_id UUID
)
RETURNS BOOLEAN
LANGUAGE plpgsql
STABLE
SET search_path = public
AS $$
DECLARE
    country_codes_arr TEXT[];
BEGIN
    SELECT array_agg(upper(trim(v))) INTO country_codes_arr
      FROM unnest(string_to_array(NULLIF(TRIM(query_params->>'country_codes'), ''), ',')) v
      WHERE length(trim(v)) = 2;

    RETURN country_codes_arr IS NULL OR EXISTS (
        SELECT 1
        FROM activities a
        WHERE a.id = activity_id
          AND a.visited_country_codes && country_codes_arr
    );
END;
$$;

DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_heatmap_modifier_filter_matches(query_params, hl.activity_id)',
        'AND skadia_heatmap_modifier_filter_matches(query_params, hl.activity_id)
          AND skadia_heatmap_country_filter_matches(query_params, hl.activity_id)'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000374 could not patch heatmap_lines_mvt country predicate';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_heatmap_modifier_filter_matches(query_params, hp.activity_id)',
        'AND skadia_heatmap_modifier_filter_matches(query_params, hp.activity_id)
          AND skadia_heatmap_country_filter_matches(query_params, hp.activity_id)'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000374 could not patch heatmap_points_mvt country predicate';
    END IF;
    EXECUTE patched;
END $$;
