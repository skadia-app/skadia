SELECT pg_advisory_xact_lock(330346, 1);

ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_sport_type_canonical;
ALTER TABLE planned_routes DROP CONSTRAINT IF EXISTS planned_routes_sport_tuple_valid;
ALTER TABLE routing_profiles DROP CONSTRAINT IF EXISTS routing_profiles_predicates_valid;
ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_allowed_predicates_valid;

CREATE OR REPLACE FUNCTION fn_validate_predicates(p JSONB)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
AS $$
WITH allowed_sports(sport) AS (
    VALUES
        ('alpine_skiing'), ('american_football'), ('backcountry_skiing'),
        ('baseball'), ('basketball'), ('boating'), ('boxing'), ('canoeing'),
        ('cricket'), ('cross_country_skiing'), ('cycling'), ('dance'),
        ('disc_golf'), ('diving'), ('driving'), ('fishing'),
        ('fitness_equipment'), ('floor_climbing'), ('flying'), ('generic'),
        ('golf'), ('hang_gliding'), ('hiit'), ('hiking'), ('hockey'),
        ('horseback_riding'), ('hunting'), ('ice_skating'), ('inline_skating'),
        ('jump_rope'), ('jumpmaster'), ('kayaking'), ('kitesurfing'),
        ('lacrosse'), ('meditation'), ('mixed_martial_arts'), ('motorcycling'),
        ('mountaineering'), ('multisport'), ('paddling'), ('pilates'),
        ('racket'), ('rafting'), ('rock_climbing'), ('rowing'), ('rugby'),
        ('running'), ('sailing'), ('skiing'), ('sky_diving'), ('sledding'),
        ('snorkeling'), ('snowboarding'), ('snowmobiling'), ('snowshoeing'),
        ('soccer'), ('stand_up_paddleboarding'), ('strength'),
        ('strength_training'), ('surfing'), ('swimming'), ('tactical'),
        ('tennis'), ('training'), ('transition'), ('unknown'), ('volleyball'),
        ('wakeboarding'), ('wakesurfing'), ('walking'), ('water_skiing'),
        ('water_tubing'), ('wheelchair_push'), ('wheelchair_push_run'),
        ('wheelchair_push_walk'), ('windsurfing'), ('yoga')
),
allowed_subtypes(sport, sub_type) AS (
    VALUES
        ('alpine_skiing','backcountry'), ('alpine_skiing','indoor_skiing'), ('alpine_skiing','resort'),
        ('cross_country_skiing','skate_skiing'),
        ('cycling','bmx'), ('cycling','cyclocross'), ('cycling','downhill'), ('cycling','gravel_cycling'),
        ('cycling','hand_cycling'), ('cycling','indoor_cycling'), ('cycling','mixed_surface'),
        ('cycling','mountain'), ('cycling','recumbent'), ('cycling','road'), ('cycling','spin'),
        ('cycling','track_cycling'), ('cycling','unicycling'),
        ('diving','apnea_diving'), ('diving','apnea_hunting'), ('diving','apnea_pool'),
        ('diving','ccr_diving'), ('diving','gauge_diving'), ('diving','multi_gas_diving'), ('diving','single_gas_diving'),
        ('fitness_equipment','elliptical'), ('fitness_equipment','indoor_cycling'),
        ('fitness_equipment','indoor_hand_cycling'), ('fitness_equipment','indoor_rowing'),
        ('fitness_equipment','indoor_skiing'), ('fitness_equipment','indoor_walking'),
        ('fitness_equipment','pilates'), ('fitness_equipment','stair_climbing'), ('fitness_equipment','treadmill'),
        ('flying','fly_altimeter'), ('flying','fly_canopy'), ('flying','fly_ifr'), ('flying','fly_navigate'),
        ('flying','fly_paraglide'), ('flying','fly_paramotor'), ('flying','fly_pressurized'),
        ('flying','fly_timer'), ('flying','fly_vfr'), ('flying','fly_wx'), ('flying','rc_drone'), ('flying','wingsuit'),
        ('hiit','tabata'), ('kayaking','whitewater'),
        ('motorcycling','atv'), ('motorcycling','motocross'),
        ('racket','badminton'), ('racket','padel'), ('racket','pickleball'), ('racket','racquetball'),
        ('racket','squash'), ('racket','table_tennis'), ('rafting','whitewater'),
        ('rock_climbing','bouldering'), ('rock_climbing','indoor_climbing'),
        ('rowing','indoor_rowing'),
        ('running','indoor_running'), ('running','obstacle'), ('running','street'), ('running','track'),
        ('running','trail'), ('running','treadmill'), ('running','ultra'),
        ('sailing','sail_race'), ('snowboarding','backcountry'), ('snowboarding','resort'),
        ('strength_training','amrap'), ('strength_training','emom'),
        ('swimming','lap_swimming'), ('swimming','open_water'),
        ('tennis','exercise'), ('tennis','match'), ('tennis','warm_up'),
        ('training','breathing'), ('training','cardio_training'), ('training','flexibility_training'),
        ('training','hiit'), ('training','pilates'), ('training','strength_training'), ('training','yoga'),
        ('transition','bike_to_run_transition'), ('transition','run_to_bike_transition'), ('transition','swim_to_bike_transition'),
        ('walking','casual_walking'), ('walking','indoor_walking'), ('walking','speed_walking'),
        ('wheelchair_push','run_speed'), ('wheelchair_push','walk_speed'),
        ('wheelchair_push_run','indoor_wheelchair_run'),
        ('wheelchair_push_walk','indoor_wheelchair_walk')
),
items AS (
    SELECT value AS pred
    FROM jsonb_array_elements(CASE WHEN jsonb_typeof(p) = 'array' THEN p ELSE '[]'::jsonb END)
)
SELECT
    COALESCE(jsonb_typeof(p) = 'array', false)
    AND NOT EXISTS (
        SELECT 1
        FROM items
        WHERE jsonb_typeof(pred) <> 'object'
           OR EXISTS (
                SELECT 1
                FROM jsonb_object_keys(pred) AS pred_keys(key)
                WHERE pred_keys.key NOT IN ('sport', 'sub_type')
           )
           OR NOT (pred ? 'sport')
           OR jsonb_typeof(pred->'sport') <> 'string'
           OR NOT EXISTS (SELECT 1 FROM allowed_sports s WHERE s.sport = pred->>'sport')
           OR (
                pred ? 'sub_type'
                AND (
                    jsonb_typeof(pred->'sub_type') <> 'string'
                    OR NOT (
                        pred->>'sub_type' = ''
                        OR EXISTS (
                            SELECT 1
                            FROM allowed_subtypes st
                            WHERE st.sport = pred->>'sport'
                              AND st.sub_type = pred->>'sub_type'
                        )
                    )
                )
           )
    );
$$;

CREATE OR REPLACE FUNCTION fp_sport_taxonomy_normalize(in_sport TEXT, in_sub_type TEXT)
RETURNS TABLE(sport_type TEXT, sub_type TEXT, is_indoor BOOLEAN, is_electric BOOLEAN)
LANGUAGE plpgsql
AS $$
DECLARE
    raw_sport TEXT := btrim(COALESCE(in_sport, ''));
    raw_sub_type TEXT := btrim(COALESCE(in_sub_type, ''));
BEGIN
    sport_type := raw_sport;
    sub_type := raw_sub_type;
    is_indoor := false;
    is_electric := false;

    CASE raw_sport
        WHEN 'bike' THEN sport_type := 'cycling'; sub_type := '';
        WHEN 'biking' THEN sport_type := 'cycling'; sub_type := '';
        WHEN 'ride' THEN sport_type := 'cycling'; sub_type := '';
        WHEN 'mountain_biking' THEN sport_type := 'cycling'; sub_type := 'mountain';
        WHEN 'gravel_cycling' THEN sport_type := 'cycling'; sub_type := 'gravel_cycling';
        WHEN 'indoor_cycling' THEN sport_type := 'cycling'; sub_type := 'indoor_cycling'; is_indoor := true;
        WHEN 'trail_running' THEN sport_type := 'running'; sub_type := 'trail';
        WHEN 'treadmill_running' THEN sport_type := 'running'; sub_type := 'treadmill'; is_indoor := true;
        WHEN 'lap_swimming' THEN sport_type := 'swimming'; sub_type := 'lap_swimming';
        WHEN 'open_water_swimming' THEN sport_type := 'swimming'; sub_type := 'open_water';
        WHEN 'road_cycling' THEN sport_type := 'cycling'; sub_type := 'road';
        WHEN 'roadcycling' THEN sport_type := 'cycling'; sub_type := 'road';
        WHEN 'wheelchair_push_walk' THEN sport_type := 'wheelchair_push'; sub_type := 'walk_speed';
        WHEN 'wheelchair_push_run' THEN sport_type := 'wheelchair_push'; sub_type := 'run_speed';
        WHEN 'strength' THEN sport_type := 'strength_training'; sub_type := '';
        WHEN 'e_mountain_biking' THEN sport_type := 'cycling'; sub_type := 'mountain'; is_electric := true;
        WHEN 'e_biking' THEN sport_type := 'cycling'; sub_type := ''; is_electric := true;
        WHEN 'e_bike' THEN sport_type := 'cycling'; sub_type := ''; is_electric := true;
        ELSE
            sport_type := raw_sport;
            sub_type := raw_sub_type;
    END CASE;

    IF raw_sport = 'wheelchair_push_walk' AND raw_sub_type = 'indoor_wheelchair_walk' THEN
        sport_type := 'wheelchair_push';
        sub_type := 'walk_speed';
        is_indoor := true;
    ELSIF raw_sport = 'wheelchair_push_run' AND raw_sub_type = 'indoor_wheelchair_run' THEN
        sport_type := 'wheelchair_push';
        sub_type := 'run_speed';
        is_indoor := true;
    ELSIF raw_sport IN ('bike', 'biking', 'ride', 'e_biking', 'e_bike') AND raw_sub_type <> '' AND raw_sub_type <> 'generic' THEN
        sub_type := raw_sub_type;
    END IF;

    IF sub_type = 'generic' THEN
        sub_type := '';
    END IF;

    IF sport_type = 'running' AND sub_type = 'treadmill' THEN
        is_indoor := true;
    END IF;
    IF sport_type = 'cycling' AND sub_type = 'indoor_cycling' THEN
        is_indoor := true;
    END IF;

    IF sub_type <> ''
       AND NOT fn_validate_predicates(jsonb_build_array(jsonb_build_object(
            'sport', sport_type,
            'sub_type', sub_type
       )))
       AND fn_validate_predicates(jsonb_build_array(jsonb_build_object(
            'sport', sport_type
       ))) THEN
        sub_type := '';
    END IF;

    IF NOT fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
        'sport', sport_type,
        'sub_type', NULLIF(sub_type, '')
    )))) THEN
        RAISE EXCEPTION 'unknown sport_type "%"', raw_sport;
    END IF;

    RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_rewrite_predicates(predicates JSONB)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    pred JSONB;
    n RECORD;
    out JSONB := '[]'::jsonb;
    rewritten JSONB;
BEGIN
    IF predicates IS NULL OR jsonb_typeof(predicates) <> 'array' THEN
        RETURN predicates;
    END IF;

    FOR pred IN SELECT value FROM jsonb_array_elements(predicates) LOOP
        IF jsonb_typeof(pred) <> 'object' OR NOT (pred ? 'sport') THEN
            out := out || jsonb_build_array(pred);
            CONTINUE;
        END IF;
        SELECT * INTO n FROM fp_sport_taxonomy_normalize(pred->>'sport', COALESCE(pred->>'sub_type', ''));
        rewritten := jsonb_build_object('sport', n.sport_type);
        IF n.sub_type <> '' THEN
            rewritten := rewritten || jsonb_build_object('sub_type', n.sub_type);
        END IF;
        out := out || jsonb_build_array(rewritten);
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_modifier_filters_from_predicates(predicates JSONB, existing JSONB)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    pred JSONB;
    n RECORD;
    out JSONB := COALESCE(existing, '{}'::jsonb);
BEGIN
    IF predicates IS NULL OR jsonb_typeof(predicates) <> 'array' THEN
        RETURN out;
    END IF;
    FOR pred IN SELECT value FROM jsonb_array_elements(predicates) LOOP
        IF jsonb_typeof(pred) = 'object' AND pred ? 'sport' THEN
            SELECT * INTO n FROM fp_sport_taxonomy_normalize(pred->>'sport', COALESCE(pred->>'sub_type', ''));
            IF n.is_indoor THEN
                out := out || jsonb_build_object('is_indoor', true);
            END IF;
            IF n.is_electric THEN
                out := out || jsonb_build_object('is_electric', true);
            END IF;
        END IF;
    END LOOP;
    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_predicates_from_sport_types(sports TEXT[])
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    raw_sport TEXT;
    n RECORD;
    pred JSONB;
    out JSONB := '[]'::jsonb;
BEGIN
    FOREACH raw_sport IN ARRAY COALESCE(sports, ARRAY[]::text[]) LOOP
        SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
        pred := jsonb_build_object('sport', n.sport_type);
        IF n.sub_type <> '' THEN
            pred := pred || jsonb_build_object('sub_type', n.sub_type);
        END IF;
        out := out || jsonb_build_array(pred);
    END LOOP;
    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_sport_types_from_predicates(predicates JSONB)
RETURNS TEXT[]
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT COALESCE(array_agg(DISTINCT pred->>'sport' ORDER BY pred->>'sport'), ARRAY[]::text[])
    FROM jsonb_array_elements(CASE WHEN jsonb_typeof(predicates) = 'array' THEN predicates ELSE '[]'::jsonb END) pred
    WHERE pred ? 'sport';
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_rewrite_conditions(conditions JSONB, surface TEXT, reject_tuple_in_or BOOLEAN)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    cond JSONB;
    out JSONB := '[]'::jsonb;
    op TEXT;
    raw_sport TEXT;
    raw_values TEXT[];
    normalized_values TEXT[];
    n RECORD;
    needs_tuple BOOLEAN;
BEGIN
    IF conditions IS NULL OR jsonb_typeof(conditions) <> 'array' THEN
        RETURN conditions;
    END IF;

    FOR cond IN SELECT value FROM jsonb_array_elements(conditions) LOOP
        IF cond->>'field' IS DISTINCT FROM 'sport_type' THEN
            out := out || jsonb_build_array(cond);
            CONTINUE;
        END IF;

        op := lower(btrim(COALESCE(cond->>'op', '')));
        IF op <> 'eq' THEN
            IF op = 'neq' THEN
                raw_sport := cond->>'value';
                SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
                IF n.sub_type <> '' OR n.is_indoor OR n.is_electric THEN
                    RAISE EXCEPTION '%: unsupported sport_type operator "%" for legacy or subtype sport predicate "%"', surface, cond->>'op', raw_sport;
                END IF;
                out := out || jsonb_build_array(jsonb_set(cond, '{value}', to_jsonb(n.sport_type), true));
                CONTINUE;
            END IF;
            IF op IN ('in', 'not_in') THEN
                IF jsonb_typeof(cond->'value') <> 'array' THEN
                    RAISE EXCEPTION '%: sport_type % condition value must be a string array', surface, cond->>'op';
                END IF;
                SELECT array_agg(value ORDER BY value)
                INTO raw_values
                FROM jsonb_array_elements_text(cond->'value') AS values(value);
                normalized_values := ARRAY[]::text[];
                FOREACH raw_sport IN ARRAY COALESCE(raw_values, ARRAY[]::text[]) LOOP
                    SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
                    IF n.sub_type <> '' OR n.is_indoor OR n.is_electric THEN
                        RAISE EXCEPTION '%: unsupported sport_type operator "%" for legacy or subtype sport predicate "%"', surface, cond->>'op', raw_sport;
                    END IF;
                    normalized_values := array_append(normalized_values, n.sport_type);
                END LOOP;
                SELECT array_agg(DISTINCT value ORDER BY value)
                INTO normalized_values
                FROM unnest(normalized_values) AS values(value);
                out := out || jsonb_build_array(jsonb_set(cond, '{value}', to_jsonb(COALESCE(normalized_values, ARRAY[]::text[])), true));
                CONTINUE;
            END IF;
            RAISE EXCEPTION '%: unsupported sport_type operator "%" for sport taxonomy rewrite', surface, cond->>'op';
        END IF;

        raw_sport := cond->>'value';
        SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
        needs_tuple := raw_sport IS DISTINCT FROM n.sport_type OR n.sub_type <> '' OR n.is_indoor OR n.is_electric;
        IF reject_tuple_in_or AND needs_tuple THEN
                RAISE EXCEPTION '%: unsupported sport_type operator "%" for legacy or subtype sport predicate "%"', surface, cond->>'op', raw_sport;
        END IF;

        out := out || jsonb_build_array(jsonb_set(cond, '{value}', to_jsonb(n.sport_type), true));
        IF n.sub_type <> '' THEN
            out := out || jsonb_build_array(jsonb_build_object('field', 'sub_type', 'op', 'eq', 'value', n.sub_type));
        END IF;
        IF n.is_indoor THEN
            out := out || jsonb_build_array(jsonb_build_object('field', 'is_indoor', 'op', 'eq', 'value', true));
        END IF;
        IF n.is_electric THEN
            out := out || jsonb_build_array(jsonb_build_object('field', 'is_electric', 'op', 'eq', 'value', true));
        END IF;
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_rewrite_condition_groups(groups JSONB, surface TEXT)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    group_item JSONB;
    out JSONB := '[]'::jsonb;
    logic TEXT;
BEGIN
    IF groups IS NULL OR jsonb_typeof(groups) <> 'array' THEN
        RETURN groups;
    END IF;
    FOR group_item IN SELECT value FROM jsonb_array_elements(groups) LOOP
        logic := lower(btrim(COALESCE(group_item->>'logic', 'and')));
        out := out || jsonb_build_array(jsonb_set(
            group_item,
            '{conditions}',
            fp_plan_t_rewrite_conditions(COALESCE(group_item->'conditions', '[]'::jsonb), surface, logic = 'or'),
            true
        ));
    END LOOP;
    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_rewrite_actions(actions JSONB, surface TEXT)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    action_item JSONB;
    out JSONB := '[]'::jsonb;
    action_name TEXT;
    params JSONB;
    n RECORD;
    new_params JSONB;
BEGIN
    IF actions IS NULL OR jsonb_typeof(actions) <> 'array' THEN
        RETURN actions;
    END IF;
    FOR action_item IN SELECT value FROM jsonb_array_elements(actions) LOOP
        action_name := COALESCE(action_item->>'action', action_item->>'type');
        IF action_name = 'set_field' AND action_item->'params'->>'field' = 'sport_type' THEN
            action_item := jsonb_build_object(
                'action', 'set_sport_type',
                'params', jsonb_build_object('sport_type', action_item->'params'->>'value')
            );
            action_name := 'set_sport_type';
        END IF;
        IF action_name IS DISTINCT FROM 'set_sport_type' OR NOT (action_item ? 'params') THEN
            out := out || jsonb_build_array(action_item);
            CONTINUE;
        END IF;

        params := action_item->'params';
        SELECT * INTO n FROM fp_sport_taxonomy_normalize(params->>'sport_type', COALESCE(params->>'sub_type', ''));
        new_params := (params || jsonb_build_object('sport_type', n.sport_type)) - 'sub_type';
        IF n.sub_type <> '' THEN
            new_params := new_params || jsonb_build_object('sub_type', n.sub_type);
        END IF;
        out := out || jsonb_build_array((action_item - 'type') || jsonb_build_object('action', 'set_sport_type', 'params', new_params));
        IF n.is_indoor THEN
            out := out || jsonb_build_array(jsonb_build_object('action', 'set_indoor', 'params', jsonb_build_object('indoor', true)));
        END IF;
        IF n.is_electric THEN
            out := out || jsonb_build_array(jsonb_build_object('action', 'set_electric', 'params', jsonb_build_object('electric', true)));
        END IF;
    END LOOP;
    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_rewrite_branches(branches JSONB)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    branch_item JSONB;
    out JSONB := '[]'::jsonb;
BEGIN
    IF branches IS NULL OR jsonb_typeof(branches) <> 'array' THEN
        RETURN branches;
    END IF;
    FOR branch_item IN SELECT value FROM jsonb_array_elements(branches) LOOP
        IF branch_item ? 'condition_groups' THEN
            branch_item := jsonb_set(branch_item, '{condition_groups}', fp_plan_t_rewrite_condition_groups(branch_item->'condition_groups', 'branches[].condition_groups'), true);
        END IF;
        IF branch_item ? 'actions' THEN
            branch_item := jsonb_set(branch_item, '{actions}', fp_plan_t_rewrite_actions(branch_item->'actions', 'branches[].actions'), true);
        END IF;
        out := out || jsonb_build_array(branch_item);
    END LOOP;
    RETURN out;
END;
$$;

WITH normalized AS (
    SELECT a.id, n.*
    FROM activities a
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(a.sport_type, COALESCE(a.sub_type, '')) n
    WHERE a.sport_type IN (
        'bike', 'biking', 'ride', 'mountain_biking', 'gravel_cycling',
        'indoor_cycling', 'trail_running', 'treadmill_running',
        'lap_swimming', 'open_water_swimming', 'road_cycling', 'roadcycling',
        'wheelchair_push_walk', 'wheelchair_push_run', 'strength',
        'e_mountain_biking', 'e_biking', 'e_bike'
    )
       OR COALESCE(a.sub_type, '') <> ''
)
UPDATE activities a
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, ''),
    is_indoor = a.is_indoor OR n.is_indoor,
    is_electric = a.is_electric OR n.is_electric
FROM normalized n
WHERE a.id = n.id
  AND (
    a.sport_type IS DISTINCT FROM n.sport_type
    OR COALESCE(a.sub_type, '') IS DISTINCT FROM n.sub_type
    OR (n.is_indoor AND NOT a.is_indoor)
    OR (n.is_electric AND NOT a.is_electric)
  );

WITH normalized AS (
    SELECT a.id, n.*
    FROM activities a
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(a.parsed_sport_type, COALESCE(a.parsed_sub_type, '')) n
    WHERE a.parsed_sport_type IS NOT NULL
      AND (
        a.parsed_sport_type IN (
            'bike', 'biking', 'ride', 'mountain_biking', 'gravel_cycling',
            'indoor_cycling', 'trail_running', 'treadmill_running',
            'lap_swimming', 'open_water_swimming', 'road_cycling', 'roadcycling',
            'wheelchair_push_walk', 'wheelchair_push_run', 'strength',
            'e_mountain_biking', 'e_biking', 'e_bike'
        )
        OR COALESCE(a.parsed_sub_type, '') <> ''
      )
)
UPDATE activities a
SET parsed_sport_type = n.sport_type,
    parsed_sub_type = NULLIF(n.sub_type, ''),
    is_indoor = a.is_indoor OR n.is_indoor,
    is_electric = a.is_electric OR n.is_electric
FROM normalized n
WHERE a.id = n.id
  AND (
    a.parsed_sport_type IS DISTINCT FROM n.sport_type
    OR COALESCE(a.parsed_sub_type, '') IS DISTINCT FROM n.sub_type
    OR (n.is_indoor AND NOT a.is_indoor)
    OR (n.is_electric AND NOT a.is_electric)
  );

WITH normalized AS (
    SELECT h.id, n.*
    FROM heatmap_lines h
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(h.sport_type, COALESCE(h.sub_type, '')) n
    WHERE h.sport_type IS NOT NULL
      AND h.sport_type IN (
            'bike', 'biking', 'ride', 'mountain_biking', 'gravel_cycling',
            'indoor_cycling', 'trail_running', 'treadmill_running',
            'lap_swimming', 'open_water_swimming', 'road_cycling', 'roadcycling',
            'wheelchair_push_walk', 'wheelchair_push_run', 'strength',
            'e_mountain_biking', 'e_biking', 'e_bike'
      )
)
UPDATE heatmap_lines h
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM normalized n
WHERE h.id = n.id
  AND (
    h.sport_type IS DISTINCT FROM n.sport_type
    OR COALESCE(h.sub_type, '') IS DISTINCT FROM n.sub_type
  );

WITH normalized AS (
    SELECT h.id, n.*
    FROM heatmap_points h
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(h.sport_type, COALESCE(h.sub_type, '')) n
    WHERE h.sport_type IS NOT NULL
      AND h.sport_type IN (
            'bike', 'biking', 'ride', 'mountain_biking', 'gravel_cycling',
            'indoor_cycling', 'trail_running', 'treadmill_running',
            'lap_swimming', 'open_water_swimming', 'road_cycling', 'roadcycling',
            'wheelchair_push_walk', 'wheelchair_push_run', 'strength',
            'e_mountain_biking', 'e_biking', 'e_bike'
      )
)
UPDATE heatmap_points h
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM normalized n
WHERE h.id = n.id
  AND (
    h.sport_type IS DISTINCT FROM n.sport_type
    OR COALESCE(h.sub_type, '') IS DISTINCT FROM n.sub_type
  );

WITH normalized AS (
    SELECT se.ctid AS row_id, n.sport_type, COALESCE(NULLIF(n.sub_type, ''), 'generic') AS sub_type
    FROM segment_efforts se
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(se.sport_type, COALESCE(se.sub_type, '')) n
    WHERE se.sport_type IN (
        'bike', 'biking', 'ride', 'mountain_biking', 'gravel_cycling',
        'indoor_cycling', 'trail_running', 'treadmill_running',
        'lap_swimming', 'open_water_swimming', 'road_cycling', 'roadcycling',
        'wheelchair_push_walk', 'wheelchair_push_run', 'strength',
        'e_mountain_biking', 'e_biking', 'e_bike'
    )
)
UPDATE segment_efforts se
SET sport_type = n.sport_type,
    sub_type = n.sub_type
FROM normalized n
WHERE se.ctid = n.row_id
  AND (
    se.sport_type IS DISTINCT FROM n.sport_type
    OR COALESCE(se.sub_type, 'generic') IS DISTINCT FROM n.sub_type
  );

WITH normalized AS (
    SELECT pr.id, n.*
    FROM planned_routes pr
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(pr.sport_type, COALESCE(pr.sub_type, '')) n
    WHERE pr.sport_type IN (
        'bike', 'biking', 'ride', 'mountain_biking', 'gravel_cycling',
        'indoor_cycling', 'trail_running', 'treadmill_running',
        'lap_swimming', 'open_water_swimming', 'road_cycling', 'roadcycling',
        'wheelchair_push_walk', 'wheelchair_push_run', 'strength',
        'e_mountain_biking', 'e_biking', 'e_bike'
    )
       OR COALESCE(pr.sub_type, '') <> ''
)
UPDATE planned_routes pr
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, ''),
    is_indoor = pr.is_indoor OR n.is_indoor,
    is_electric = pr.is_electric OR n.is_electric
FROM normalized n
WHERE pr.id = n.id
  AND (
    pr.sport_type IS DISTINCT FROM n.sport_type
    OR COALESCE(pr.sub_type, '') IS DISTINCT FROM n.sub_type
    OR (n.is_indoor AND NOT pr.is_indoor)
    OR (n.is_electric AND NOT pr.is_electric)
  );

WITH normalized AS (
    SELECT rg.id, n.*
    FROM recording_groups rg
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(rg.sport_type, COALESCE(rg.sub_type, '')) n
    WHERE rg.sport_type IS NOT NULL
      AND (
        rg.sport_type IN (
            'bike', 'biking', 'ride', 'mountain_biking', 'gravel_cycling',
            'indoor_cycling', 'trail_running', 'treadmill_running',
            'lap_swimming', 'open_water_swimming', 'road_cycling', 'roadcycling',
            'wheelchair_push_walk', 'wheelchair_push_run', 'strength',
            'e_mountain_biking', 'e_biking', 'e_bike'
        )
        OR COALESCE(rg.sub_type, '') <> ''
      )
)
UPDATE recording_groups rg
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, ''),
    is_indoor = rg.is_indoor OR n.is_indoor,
    is_electric = rg.is_electric OR n.is_electric
FROM normalized n
WHERE rg.id = n.id
  AND (
    rg.sport_type IS DISTINCT FROM n.sport_type
    OR COALESCE(rg.sub_type, '') IS DISTINCT FROM n.sub_type
    OR (n.is_indoor AND NOT rg.is_indoor)
    OR (n.is_electric AND NOT rg.is_electric)
  );

UPDATE segments
SET allowed_predicates = fp_plan_t_rewrite_predicates(allowed_predicates)
WHERE allowed_predicates IS NOT NULL;

WITH rewritten_profiles AS (
    SELECT
        rp.id,
        CASE
            WHEN jsonb_typeof(rp.predicates) = 'array' AND jsonb_array_length(rp.predicates) > 0
                THEN fp_plan_t_rewrite_predicates(rp.predicates)
            WHEN COALESCE(array_length(rp.sport_types, 1), 0) > 0
                THEN fp_plan_t_predicates_from_sport_types(rp.sport_types)
            ELSE rp.predicates
        END AS predicates
    FROM routing_profiles rp
)
UPDATE routing_profiles rp
SET predicates = rewritten.predicates,
    modifier_filters = fp_plan_t_modifier_filters_from_predicates(rewritten.predicates, rp.modifier_filters)
FROM rewritten_profiles rewritten
WHERE rp.id = rewritten.id;

UPDATE routing_profiles
SET sport_types = fp_plan_t_sport_types_from_predicates(predicates);

UPDATE automation_rules
SET conditions = fp_plan_t_rewrite_conditions(conditions, 'conditions', false),
    condition_groups = fp_plan_t_rewrite_condition_groups(condition_groups, 'condition_groups'),
    actions = fp_plan_t_rewrite_actions(actions, 'actions'),
    branches = fp_plan_t_rewrite_branches(branches);

CREATE OR REPLACE FUNCTION fn_validate_predicates(p JSONB)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
AS $$
WITH allowed_sports(sport) AS (
    VALUES
        ('alpine_skiing'), ('american_football'), ('backcountry_skiing'),
        ('baseball'), ('basketball'), ('boating'), ('boxing'), ('canoeing'),
        ('cricket'), ('cross_country_skiing'), ('cycling'), ('dance'),
        ('disc_golf'), ('diving'), ('driving'), ('fishing'),
        ('fitness_equipment'), ('floor_climbing'), ('flying'), ('generic'),
        ('golf'), ('hang_gliding'), ('hiit'), ('hiking'), ('hockey'),
        ('horseback_riding'), ('hunting'), ('ice_skating'), ('inline_skating'),
        ('jump_rope'), ('jumpmaster'), ('kayaking'), ('kitesurfing'),
        ('lacrosse'), ('meditation'), ('mixed_martial_arts'), ('motorcycling'),
        ('mountaineering'), ('multisport'), ('paddling'), ('pilates'),
        ('racket'), ('rafting'), ('rock_climbing'), ('rowing'), ('rugby'),
        ('running'), ('sailing'), ('skiing'), ('sky_diving'), ('sledding'),
        ('snorkeling'), ('snowboarding'), ('snowmobiling'), ('snowshoeing'),
        ('soccer'), ('stand_up_paddleboarding'), ('strength_training'),
        ('surfing'), ('swimming'), ('tactical'), ('tennis'), ('training'),
        ('transition'), ('unknown'), ('volleyball'), ('wakeboarding'),
        ('wakesurfing'), ('walking'), ('water_skiing'), ('water_tubing'),
        ('wheelchair_push'), ('windsurfing'), ('yoga')
),
allowed_subtypes(sport, sub_type) AS (
    VALUES
        ('alpine_skiing','backcountry'), ('alpine_skiing','indoor_skiing'), ('alpine_skiing','resort'),
        ('cross_country_skiing','skate_skiing'),
        ('cycling','bmx'), ('cycling','cyclocross'), ('cycling','downhill'), ('cycling','gravel_cycling'),
        ('cycling','hand_cycling'), ('cycling','indoor_cycling'), ('cycling','mixed_surface'),
        ('cycling','mountain'), ('cycling','recumbent'), ('cycling','road'), ('cycling','spin'),
        ('cycling','track_cycling'), ('cycling','unicycling'),
        ('diving','apnea_diving'), ('diving','apnea_hunting'), ('diving','apnea_pool'),
        ('diving','ccr_diving'), ('diving','gauge_diving'), ('diving','multi_gas_diving'), ('diving','single_gas_diving'),
        ('fitness_equipment','elliptical'), ('fitness_equipment','indoor_cycling'),
        ('fitness_equipment','indoor_hand_cycling'), ('fitness_equipment','indoor_rowing'),
        ('fitness_equipment','indoor_skiing'), ('fitness_equipment','indoor_walking'),
        ('fitness_equipment','pilates'), ('fitness_equipment','stair_climbing'), ('fitness_equipment','treadmill'),
        ('flying','fly_altimeter'), ('flying','fly_canopy'), ('flying','fly_ifr'), ('flying','fly_navigate'),
        ('flying','fly_paraglide'), ('flying','fly_paramotor'), ('flying','fly_pressurized'),
        ('flying','fly_timer'), ('flying','fly_vfr'), ('flying','fly_wx'), ('flying','rc_drone'), ('flying','wingsuit'),
        ('hiit','tabata'), ('kayaking','whitewater'),
        ('motorcycling','atv'), ('motorcycling','motocross'),
        ('racket','badminton'), ('racket','padel'), ('racket','pickleball'), ('racket','racquetball'),
        ('racket','squash'), ('racket','table_tennis'), ('rafting','whitewater'),
        ('rock_climbing','bouldering'), ('rock_climbing','indoor_climbing'),
        ('rowing','indoor_rowing'),
        ('running','indoor_running'), ('running','obstacle'), ('running','street'), ('running','track'),
        ('running','trail'), ('running','treadmill'), ('running','ultra'),
        ('sailing','sail_race'), ('snowboarding','backcountry'), ('snowboarding','resort'),
        ('strength_training','amrap'), ('strength_training','emom'),
        ('swimming','lap_swimming'), ('swimming','open_water'),
        ('tennis','exercise'), ('tennis','match'), ('tennis','warm_up'),
        ('training','breathing'), ('training','cardio_training'), ('training','flexibility_training'),
        ('training','hiit'), ('training','pilates'), ('training','strength_training'), ('training','yoga'),
        ('transition','bike_to_run_transition'), ('transition','run_to_bike_transition'), ('transition','swim_to_bike_transition'),
        ('walking','casual_walking'), ('walking','indoor_walking'), ('walking','speed_walking'),
        ('wheelchair_push','run_speed'), ('wheelchair_push','walk_speed')
),
items AS (
    SELECT value AS pred
    FROM jsonb_array_elements(CASE WHEN jsonb_typeof(p) = 'array' THEN p ELSE '[]'::jsonb END)
)
SELECT
    COALESCE(jsonb_typeof(p) = 'array', false)
    AND NOT EXISTS (
        SELECT 1
        FROM items
        WHERE jsonb_typeof(pred) <> 'object'
           OR EXISTS (
                SELECT 1
                FROM jsonb_object_keys(pred) AS pred_keys(key)
                WHERE pred_keys.key NOT IN ('sport', 'sub_type')
           )
           OR NOT (pred ? 'sport')
           OR jsonb_typeof(pred->'sport') <> 'string'
           OR NOT EXISTS (SELECT 1 FROM allowed_sports s WHERE s.sport = pred->>'sport')
           OR (
                pred ? 'sub_type'
                AND (
                    jsonb_typeof(pred->'sub_type') <> 'string'
                    OR NOT (
                        pred->>'sub_type' = ''
                        OR EXISTS (
                            SELECT 1
                            FROM allowed_subtypes st
                            WHERE st.sport = pred->>'sport'
                              AND st.sub_type = pred->>'sub_type'
                        )
                    )
                )
           )
    );
$$;

ALTER TABLE routing_profiles
    DROP CONSTRAINT IF EXISTS routing_profiles_predicates_valid,
    ADD CONSTRAINT routing_profiles_predicates_valid
        CHECK (jsonb_array_length(predicates) > 0 AND fn_validate_predicates(predicates));

ALTER TABLE planned_routes
    DROP CONSTRAINT IF EXISTS planned_routes_sport_tuple_valid,
    ADD CONSTRAINT planned_routes_sport_tuple_valid
        CHECK (fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
            'sport', sport_type,
            'sub_type', sub_type
        ))))) NOT VALID;
ALTER TABLE planned_routes VALIDATE CONSTRAINT planned_routes_sport_tuple_valid;

ALTER TABLE activities
    DROP CONSTRAINT IF EXISTS activities_sport_type_canonical,
    ADD CONSTRAINT activities_sport_type_canonical CHECK (
        fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
            'sport', sport_type,
            'sub_type', sub_type
        ))))
    ) NOT VALID;
ALTER TABLE activities VALIDATE CONSTRAINT activities_sport_type_canonical;

ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_allowed_predicates_valid,
    ADD CONSTRAINT segments_allowed_predicates_valid
        CHECK (allowed_predicates IS NULL OR fn_validate_predicates(allowed_predicates)) NOT VALID;
ALTER TABLE segments VALIDATE CONSTRAINT segments_allowed_predicates_valid;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM activities
        WHERE sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR sub_type IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
           OR parsed_sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR parsed_sub_type IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
    ) OR EXISTS (
        SELECT 1 FROM planned_routes
        WHERE sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR sub_type IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
    ) OR EXISTS (
        SELECT 1 FROM recording_groups
        WHERE sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR sub_type IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
    ) OR EXISTS (
        SELECT 1 FROM heatmap_lines
        WHERE sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR sub_type IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
    ) OR EXISTS (
        SELECT 1 FROM heatmap_points
        WHERE sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR sub_type IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
    ) OR EXISTS (
        SELECT 1 FROM segment_efforts
        WHERE sport_type IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR sub_type IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
    ) OR EXISTS (
        SELECT 1 FROM segments s, jsonb_array_elements(COALESCE(s.allowed_predicates, '[]'::jsonb)) pred
        WHERE pred->>'sport' IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR pred->>'sub_type' IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
    ) OR EXISTS (
        SELECT 1 FROM routing_profiles rp, jsonb_array_elements(COALESCE(rp.predicates, '[]'::jsonb)) pred
        WHERE pred->>'sport' IN ('wheelchair_push_walk', 'wheelchair_push_run', 'strength')
           OR pred->>'sub_type' IN ('indoor_wheelchair_walk', 'indoor_wheelchair_run')
           OR rp.sport_types && ARRAY['wheelchair_push_walk', 'wheelchair_push_run', 'strength']::text[]
    ) OR EXISTS (
        SELECT 1 FROM automation_rules
        WHERE conditions::text LIKE '%"wheelchair_push_walk"%'
           OR conditions::text LIKE '%"wheelchair_push_run"%'
           OR conditions::text LIKE '%"indoor_wheelchair_walk"%'
           OR conditions::text LIKE '%"indoor_wheelchair_run"%'
           OR conditions::text LIKE '%"strength"%'
           OR condition_groups::text LIKE '%"wheelchair_push_walk"%'
           OR condition_groups::text LIKE '%"wheelchair_push_run"%'
           OR condition_groups::text LIKE '%"indoor_wheelchair_walk"%'
           OR condition_groups::text LIKE '%"indoor_wheelchair_run"%'
           OR condition_groups::text LIKE '%"strength"%'
           OR actions::text LIKE '%"wheelchair_push_walk"%'
           OR actions::text LIKE '%"wheelchair_push_run"%'
           OR actions::text LIKE '%"indoor_wheelchair_walk"%'
           OR actions::text LIKE '%"indoor_wheelchair_run"%'
           OR actions::text LIKE '%"strength"%'
           OR branches::text LIKE '%"wheelchair_push_walk"%'
           OR branches::text LIKE '%"wheelchair_push_run"%'
           OR branches::text LIKE '%"indoor_wheelchair_walk"%'
           OR branches::text LIKE '%"indoor_wheelchair_run"%'
           OR branches::text LIKE '%"strength"%'
    ) THEN
        RAISE EXCEPTION 'Plan T sport taxonomy migration left removed sport values behind';
    END IF;
END;
$$;

DROP FUNCTION IF EXISTS fp_plan_t_rewrite_branches(JSONB);
DROP FUNCTION IF EXISTS fp_plan_t_rewrite_actions(JSONB, TEXT);
DROP FUNCTION IF EXISTS fp_plan_t_rewrite_condition_groups(JSONB, TEXT);
DROP FUNCTION IF EXISTS fp_plan_t_rewrite_conditions(JSONB, TEXT, BOOLEAN);
DROP FUNCTION IF EXISTS fp_plan_t_sport_types_from_predicates(JSONB);
DROP FUNCTION IF EXISTS fp_plan_t_predicates_from_sport_types(TEXT[]);
DROP FUNCTION IF EXISTS fp_plan_t_modifier_filters_from_predicates(JSONB, JSONB);
DROP FUNCTION IF EXISTS fp_plan_t_rewrite_predicates(JSONB);
DROP FUNCTION IF EXISTS fp_sport_taxonomy_normalize(TEXT, TEXT);
