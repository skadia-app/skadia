DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_heatmap_country_filter_matches(query_params, hl.activity_id)',
        ''
    );
    IF position('skadia_heatmap_country_filter_matches(query_params, hl.activity_id)' in patched) > 0 THEN
        RAISE EXCEPTION '000374 could not remove heatmap_lines_mvt country predicate';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_heatmap_country_filter_matches(query_params, hp.activity_id)',
        ''
    );
    IF position('skadia_heatmap_country_filter_matches(query_params, hp.activity_id)' in patched) > 0 THEN
        RAISE EXCEPTION '000374 could not remove heatmap_points_mvt country predicate';
    END IF;
    EXECUTE patched;
END $$;

DROP FUNCTION IF EXISTS skadia_heatmap_country_filter_matches(JSON, UUID);
