-- 000349_sport_taxonomy_predicate_validation.down.sql
-- Restore the fn_validate_predicates() body that existed before Plan E.

SELECT pg_advisory_xact_lock(hashtext('sport_taxonomy_predicate_validation'), 1);

ALTER TABLE routing_profiles DROP CONSTRAINT IF EXISTS routing_profiles_predicates_valid;
ALTER TABLE planned_routes DROP CONSTRAINT IF EXISTS planned_routes_sport_tuple_valid;
ALTER TABLE activities DROP CONSTRAINT IF EXISTS activities_sport_type_canonical;
ALTER TABLE segments DROP CONSTRAINT IF EXISTS segments_allowed_predicates_valid;
DROP FUNCTION IF EXISTS fn_validate_segment_predicates_for_type(TEXT, JSONB);

CREATE OR REPLACE FUNCTION fp_000349_down_tuple(in_sport TEXT, in_sub_type TEXT)
RETURNS TABLE(sport_type TEXT, sub_type TEXT)
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    raw_sport TEXT := btrim(COALESCE(in_sport, ''));
    raw_sub_type TEXT := btrim(COALESCE(in_sub_type, ''));
BEGIN
    sport_type := raw_sport;
    sub_type := raw_sub_type;

    IF raw_sport IN ('archery', 'geocaching', 'mobility', 'shooting', 'skateboarding') THEN
        sport_type := 'unknown';
        sub_type := '';
    ELSIF raw_sport = 'diving' AND raw_sub_type = 'dynamic_apnea' THEN
        sub_type := '';
    ELSIF raw_sport = 'hiking' AND raw_sub_type = 'rucking' THEN
        sub_type := '';
    END IF;

    RETURN NEXT;
END;
$$;

CREATE OR REPLACE FUNCTION fp_000349_down_rewrite_predicates(predicates JSONB)
RETURNS JSONB
LANGUAGE plpgsql
SET search_path = public
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
        SELECT * INTO n FROM fp_000349_down_tuple(pred->>'sport', COALESCE(pred->>'sub_type', ''));
        rewritten := pred || jsonb_build_object('sport', n.sport_type);
        IF n.sub_type = '' THEN
            rewritten := rewritten - 'sub_type';
        ELSE
            rewritten := rewritten || jsonb_build_object('sub_type', n.sub_type);
        END IF;
        out := out || jsonb_build_array(rewritten);
    END LOOP;

    RETURN out;
END;
$$;

UPDATE activities a
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM (
    SELECT a.id, n.sport_type, n.sub_type
    FROM activities a
    CROSS JOIN LATERAL fp_000349_down_tuple(a.sport_type, COALESCE(a.sub_type, '')) n
    WHERE a.sport_type IS DISTINCT FROM n.sport_type
       OR COALESCE(a.sub_type, '') IS DISTINCT FROM n.sub_type
) n
WHERE a.id = n.id;

UPDATE activities a
SET parsed_sport_type = n.sport_type,
    parsed_sub_type = NULLIF(n.sub_type, '')
FROM (
    SELECT a.id, n.sport_type, n.sub_type
    FROM activities a
    CROSS JOIN LATERAL fp_000349_down_tuple(a.parsed_sport_type, COALESCE(a.parsed_sub_type, '')) n
    WHERE a.parsed_sport_type IS NOT NULL
      AND (
        a.parsed_sport_type IS DISTINCT FROM n.sport_type
        OR COALESCE(a.parsed_sub_type, '') IS DISTINCT FROM n.sub_type
      )
) n
WHERE a.id = n.id;

UPDATE planned_routes pr
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM (
    SELECT pr.id, n.sport_type, n.sub_type
    FROM planned_routes pr
    CROSS JOIN LATERAL fp_000349_down_tuple(pr.sport_type, COALESCE(pr.sub_type, '')) n
    WHERE pr.sport_type IS DISTINCT FROM n.sport_type
       OR COALESCE(pr.sub_type, '') IS DISTINCT FROM n.sub_type
) n
WHERE pr.id = n.id;

UPDATE recording_groups rg
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM (
    SELECT rg.id, n.sport_type, n.sub_type
    FROM recording_groups rg
    CROSS JOIN LATERAL fp_000349_down_tuple(rg.sport_type, COALESCE(rg.sub_type, '')) n
    WHERE rg.sport_type IS DISTINCT FROM n.sport_type
       OR COALESCE(rg.sub_type, '') IS DISTINCT FROM n.sub_type
) n
WHERE rg.id = n.id;

UPDATE heatmap_lines h
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM (
    SELECT h.id, n.sport_type, n.sub_type
    FROM heatmap_lines h
    CROSS JOIN LATERAL fp_000349_down_tuple(h.sport_type, COALESCE(h.sub_type, '')) n
    WHERE h.sport_type IS DISTINCT FROM n.sport_type
       OR COALESCE(h.sub_type, '') IS DISTINCT FROM n.sub_type
) n
WHERE h.id = n.id;

UPDATE heatmap_points h
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM (
    SELECT h.id, n.sport_type, n.sub_type
    FROM heatmap_points h
    CROSS JOIN LATERAL fp_000349_down_tuple(h.sport_type, COALESCE(h.sub_type, '')) n
    WHERE h.sport_type IS DISTINCT FROM n.sport_type
       OR COALESCE(h.sub_type, '') IS DISTINCT FROM n.sub_type
) n
WHERE h.id = n.id;

UPDATE segment_efforts se
SET sport_type = n.sport_type,
    sub_type = COALESCE(NULLIF(n.sub_type, ''), 'generic')
FROM (
    SELECT se.id, se.started_at, n.sport_type, n.sub_type
    FROM segment_efforts se
    CROSS JOIN LATERAL fp_000349_down_tuple(se.sport_type, COALESCE(se.sub_type, '')) n
    WHERE se.sport_type IS DISTINCT FROM n.sport_type
       OR COALESCE(se.sub_type, '') IS DISTINCT FROM n.sub_type
) n
WHERE se.id = n.id
  AND se.started_at = n.started_at;

UPDATE routing_profiles
SET predicates = fp_000349_down_rewrite_predicates(predicates)
WHERE predicates IS NOT NULL;

UPDATE segments
SET allowed_predicates = fp_000349_down_rewrite_predicates(allowed_predicates)
WHERE allowed_predicates IS NOT NULL;

CREATE OR REPLACE FUNCTION fp_000349_down_rewrite_sport_tuple_rows(rows JSONB)
RETURNS JSONB
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    row_item JSONB;
    n RECORD;
    out JSONB := '[]'::jsonb;
    rewritten JSONB;
BEGIN
    IF rows IS NULL OR jsonb_typeof(rows) <> 'array' THEN
        RETURN rows;
    END IF;

    FOR row_item IN SELECT value FROM jsonb_array_elements(rows) LOOP
        IF jsonb_typeof(row_item) <> 'object' OR NOT (row_item ? 'sport_type') THEN
            out := out || jsonb_build_array(row_item);
            CONTINUE;
        END IF;

        SELECT * INTO n FROM fp_000349_down_tuple(row_item->>'sport_type', COALESCE(row_item->>'sub_type', ''));
        rewritten := row_item || jsonb_build_object('sport_type', n.sport_type);
        IF n.sub_type = '' THEN
            rewritten := rewritten - 'sub_type';
        ELSE
            rewritten := rewritten || jsonb_build_object('sub_type', n.sub_type);
        END IF;

        out := out || jsonb_build_array(rewritten);
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_000349_down_rewrite_rule_conditions(conditions JSONB)
RETURNS JSONB
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    condition_item JSONB;
    out JSONB := '[]'::jsonb;
BEGIN
    IF conditions IS NULL OR jsonb_typeof(conditions) <> 'array' THEN
        RETURN conditions;
    END IF;

    FOR condition_item IN SELECT value FROM jsonb_array_elements(conditions) LOOP
        IF jsonb_typeof(condition_item) = 'object'
           AND condition_item->>'field' = 'sport_tuple'
           AND jsonb_typeof(condition_item->'value') = 'array' THEN
            condition_item := jsonb_set(condition_item, '{value}', fp_000349_down_rewrite_sport_tuple_rows(condition_item->'value'), true);
        END IF;
        out := out || jsonb_build_array(condition_item);
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_000349_down_rewrite_rule_condition_groups(groups JSONB)
RETURNS JSONB
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    group_item JSONB;
    out JSONB := '[]'::jsonb;
BEGIN
    IF groups IS NULL OR jsonb_typeof(groups) <> 'array' THEN
        RETURN groups;
    END IF;

    FOR group_item IN SELECT value FROM jsonb_array_elements(groups) LOOP
        IF jsonb_typeof(group_item) = 'object' AND group_item ? 'conditions' THEN
            group_item := jsonb_set(group_item, '{conditions}', fp_000349_down_rewrite_rule_conditions(group_item->'conditions'), true);
        END IF;
        out := out || jsonb_build_array(group_item);
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_000349_down_rewrite_rule_actions(actions JSONB)
RETURNS JSONB
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    action_item JSONB;
    params JSONB;
    n RECORD;
    out JSONB := '[]'::jsonb;
BEGIN
    IF actions IS NULL OR jsonb_typeof(actions) <> 'array' THEN
        RETURN actions;
    END IF;

    FOR action_item IN SELECT value FROM jsonb_array_elements(actions) LOOP
        IF jsonb_typeof(action_item) = 'object'
           AND action_item->>'action' = 'set_sport_type'
           AND jsonb_typeof(action_item->'params') = 'object'
           AND action_item->'params' ? 'sport_type' THEN
            params := action_item->'params';
            SELECT * INTO n FROM fp_000349_down_tuple(params->>'sport_type', COALESCE(params->>'sub_type', ''));
            params := params || jsonb_build_object('sport_type', n.sport_type);
            IF n.sub_type = '' THEN
                params := params - 'sub_type';
            ELSE
                params := params || jsonb_build_object('sub_type', n.sub_type);
            END IF;
            action_item := jsonb_set(action_item, '{params}', params, true);
        END IF;

        out := out || jsonb_build_array(action_item);
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_000349_down_rewrite_rule_branches(branches JSONB)
RETURNS JSONB
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    branch_item JSONB;
    out JSONB := '[]'::jsonb;
BEGIN
    IF branches IS NULL OR jsonb_typeof(branches) <> 'array' THEN
        RETURN branches;
    END IF;

    FOR branch_item IN SELECT value FROM jsonb_array_elements(branches) LOOP
        IF jsonb_typeof(branch_item) = 'object' AND branch_item ? 'condition_groups' THEN
            branch_item := jsonb_set(branch_item, '{condition_groups}', fp_000349_down_rewrite_rule_condition_groups(branch_item->'condition_groups'), true);
        END IF;
        IF jsonb_typeof(branch_item) = 'object' AND branch_item ? 'actions' THEN
            branch_item := jsonb_set(branch_item, '{actions}', fp_000349_down_rewrite_rule_actions(branch_item->'actions'), true);
        END IF;
        out := out || jsonb_build_array(branch_item);
    END LOOP;

    RETURN out;
END;
$$;

UPDATE automation_rules
SET conditions = fp_000349_down_rewrite_rule_conditions(conditions),
    condition_groups = fp_000349_down_rewrite_rule_condition_groups(condition_groups),
    actions = fp_000349_down_rewrite_rule_actions(actions),
    branches = fp_000349_down_rewrite_rule_branches(branches);

DROP FUNCTION IF EXISTS fp_000349_down_rewrite_rule_branches(JSONB);
DROP FUNCTION IF EXISTS fp_000349_down_rewrite_rule_actions(JSONB);
DROP FUNCTION IF EXISTS fp_000349_down_rewrite_rule_condition_groups(JSONB);
DROP FUNCTION IF EXISTS fp_000349_down_rewrite_rule_conditions(JSONB);
DROP FUNCTION IF EXISTS fp_000349_down_rewrite_sport_tuple_rows(JSONB);
DROP FUNCTION IF EXISTS fp_000349_down_rewrite_predicates(JSONB);
DROP FUNCTION IF EXISTS fp_000349_down_tuple(TEXT, TEXT);

CREATE OR REPLACE FUNCTION fn_validate_predicates(p JSONB)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
SET search_path = public
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
    ADD CONSTRAINT routing_profiles_predicates_valid
        CHECK (jsonb_array_length(predicates) > 0 AND fn_validate_predicates(predicates));

ALTER TABLE planned_routes
    ADD CONSTRAINT planned_routes_sport_tuple_valid
        CHECK (fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
            'sport', sport_type,
            'sub_type', sub_type
        ))))) NOT VALID;
ALTER TABLE planned_routes VALIDATE CONSTRAINT planned_routes_sport_tuple_valid;

ALTER TABLE activities
    ADD CONSTRAINT activities_sport_type_canonical CHECK (
        fn_validate_predicates(jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
            'sport', sport_type,
            'sub_type', sub_type
        ))))
    ) NOT VALID;
ALTER TABLE activities VALIDATE CONSTRAINT activities_sport_type_canonical;

ALTER TABLE segments
    ADD CONSTRAINT segments_allowed_predicates_valid
        CHECK (allowed_predicates IS NULL OR fn_validate_predicates(allowed_predicates)) NOT VALID;
ALTER TABLE segments VALIDATE CONSTRAINT segments_allowed_predicates_valid;
