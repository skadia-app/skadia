DO $$
DECLARE
    fn TEXT;
    patched TEXT;
BEGIN
    SELECT pg_get_functiondef('heatmap_lines_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(include_gear_arr) AND aga.role_key = ''primary''',
        'SELECT id FROM activities WHERE gear_id = ANY(include_gear_arr)'
    );
    patched := replace(
        patched,
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(exclude_gear_arr) AND aga.role_key = ''primary''',
        'SELECT id FROM activities WHERE gear_id = ANY(exclude_gear_arr)'
    );
    IF patched = fn AND position('FROM activity_gear_assignments aga' in fn) > 0 THEN
        RAISE EXCEPTION '000372 could not unpatch heatmap_lines_mvt gear predicates';
    END IF;
    EXECUTE patched;

    SELECT pg_get_functiondef('heatmap_points_mvt(integer,integer,integer,json)'::regprocedure) INTO fn;
    patched := replace(
        fn,
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(include_gear_arr) AND aga.role_key = ''primary''',
        'SELECT id FROM activities WHERE gear_id = ANY(include_gear_arr)'
    );
    patched := replace(
        patched,
        'SELECT aga.activity_id FROM activity_gear_assignments aga WHERE aga.gear_id = ANY(exclude_gear_arr) AND aga.role_key = ''primary''',
        'SELECT id FROM activities WHERE gear_id = ANY(exclude_gear_arr)'
    );
    IF patched = fn AND position('FROM activity_gear_assignments aga' in fn) > 0 THEN
        RAISE EXCEPTION '000372 could not unpatch heatmap_points_mvt gear predicates';
    END IF;
    EXECUTE patched;
END $$;
