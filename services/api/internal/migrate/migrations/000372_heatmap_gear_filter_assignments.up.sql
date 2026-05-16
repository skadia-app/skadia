-- Route Heatmap/Tracks gear filters through activity_gear_assignments.
-- activities.gear_id was removed by the gear item migration; MVT functions must
-- use the primary assignment table instead.

DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'SELECT id FROM activities WHERE gear_id = ANY(include_gear_arr)',
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(include_gear_arr) AND aga.role_key = ''primary'''
    );
    patched := replace(
        patched,
        'SELECT id FROM activities WHERE gear_id = ANY(exclude_gear_arr)',
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(exclude_gear_arr) AND aga.role_key = ''primary'''
    );
    IF patched = fn AND position('FROM activities WHERE gear_id' in fn) > 0 THEN
        RAISE EXCEPTION '000372 could not patch heatmap_lines_mvt gear predicates';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'SELECT id FROM activities WHERE gear_id = ANY(include_gear_arr)',
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(include_gear_arr) AND aga.role_key = ''primary'''
    );
    patched := replace(
        patched,
        'SELECT id FROM activities WHERE gear_id = ANY(exclude_gear_arr)',
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(exclude_gear_arr) AND aga.role_key = ''primary'''
    );
    IF patched = fn AND position('FROM activities WHERE gear_id' in fn) > 0 THEN
        RAISE EXCEPTION '000372 could not patch heatmap_points_mvt gear predicates';
    END IF;
    EXECUTE patched;
END $$;
