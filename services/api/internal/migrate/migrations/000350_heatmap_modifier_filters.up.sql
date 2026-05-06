-- 000350_heatmap_modifier_filters.up.sql
-- Adds activity modifier filtering to the tuple-aware heatmap tile functions.

CREATE OR REPLACE FUNCTION skadia_heatmap_modifier_filter_matches(
    query_params JSON,
    activity_id UUID
)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
AS $$
    WITH filters AS (
        SELECT
            NULLIF(TRIM(query_params->>'is_electric'), '') AS is_electric,
            NULLIF(TRIM(query_params->>'is_commute'), '') AS is_commute
    )
    SELECT
        (filters.is_electric IS NULL AND filters.is_commute IS NULL)
        OR EXISTS (
            SELECT 1
            FROM activities a
            WHERE a.id = activity_id
              AND (filters.is_electric IS NULL OR a.is_electric = filters.is_electric::BOOLEAN)
              AND (filters.is_commute IS NULL OR a.is_commute = filters.is_commute::BOOLEAN)
        )
    FROM filters;
$$;

DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)',
        'AND skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)
          AND skadia_heatmap_modifier_filter_matches(query_params, hl.activity_id)'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000350 could not patch heatmap_lines_mvt modifier predicate';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)',
        'AND skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)
          AND skadia_heatmap_modifier_filter_matches(query_params, hp.activity_id)'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000350 could not patch heatmap_points_mvt modifier predicate';
    END IF;
    EXECUTE patched;
END $$;
