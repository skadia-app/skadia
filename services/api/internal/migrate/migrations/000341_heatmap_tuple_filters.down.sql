DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)',
        'AND (sport_filter     IS NULL OR hl.sport_type = ANY(string_to_array(sport_filter,    '','')))'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000341 could not unpatch heatmap_lines_mvt sport_filter predicate';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)',
        'AND (sport_filter     IS NULL OR hp.sport_type = ANY(string_to_array(sport_filter,    '','')))'
    );
    IF patched = fn THEN
        RAISE EXCEPTION '000341 could not unpatch heatmap_points_mvt sport_filter predicate';
    END IF;
    EXECUTE patched;
END $$;

DROP FUNCTION IF EXISTS skadia_sport_tuple_filter_matches(TEXT, TEXT, TEXT);
