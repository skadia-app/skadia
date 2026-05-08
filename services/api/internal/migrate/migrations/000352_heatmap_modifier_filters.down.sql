-- 000352_heatmap_modifier_filters.down.sql

DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)
          AND skadia_heatmap_modifier_filter_matches(query_params, hl.activity_id)',
        'AND skadia_sport_tuple_filter_matches(sport_filter, hl.sport_type, hl.sub_type)'
    );
    IF patched = fn AND position('skadia_heatmap_modifier_filter_matches(query_params, hl.activity_id)' in fn) > 0 THEN
        RAISE EXCEPTION '000352 could not unpatch heatmap_lines_mvt modifier predicate';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'AND skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)
          AND skadia_heatmap_modifier_filter_matches(query_params, hp.activity_id)',
        'AND skadia_sport_tuple_filter_matches(sport_filter, hp.sport_type, hp.sub_type)'
    );
    IF patched = fn AND position('skadia_heatmap_modifier_filter_matches(query_params, hp.activity_id)' in fn) > 0 THEN
        RAISE EXCEPTION '000352 could not unpatch heatmap_points_mvt modifier predicate';
    END IF;
    EXECUTE patched;
END $$;

DROP FUNCTION IF EXISTS skadia_heatmap_modifier_filter_matches(JSON, UUID);
