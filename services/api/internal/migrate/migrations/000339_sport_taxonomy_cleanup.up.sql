-- 000339_sport_taxonomy_cleanup.up.sql
--
-- Transition notes:
-- - Segment allowed_sport_types collapse broadens matching for legacy subtype-only
--   sports. Apply this migration while Wave segment predicates ship in the same
--   deployment window, or run the paired preflight and block when it reports
--   affected rows.
-- - segments_legacy_sub_type_hints is a temporary Wave-owned handoff table.
-- - Pause the rules engine during the migration window by holding the agreed
--   advisory lock from the operator runbook.
-- - The migration runner wraps each migration in one transaction, so this file
--   intentionally contains no CREATE INDEX CONCURRENTLY.
-- - Temporary partial indexes and tag-counter recomputation belong to the
--   operator runbook, not to 000339.
-- - Modifier tag synchronization is owned by existing triggers; this migration
--   updates modifier columns only.

SELECT pg_advisory_xact_lock(330339, 1);

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
        WHEN 'e_mountain_biking' THEN sport_type := 'cycling'; sub_type := 'mountain'; is_electric := true;
        WHEN 'e_biking' THEN sport_type := 'cycling'; sub_type := ''; is_electric := true;
        WHEN 'e_bike' THEN sport_type := 'cycling'; sub_type := ''; is_electric := true;
        ELSE
            sport_type := raw_sport;
            sub_type := raw_sub_type;
    END CASE;

    IF raw_sport IN (
        'mountain_biking', 'gravel_cycling', 'indoor_cycling',
        'trail_running', 'treadmill_running',
        'lap_swimming', 'open_water_swimming',
        'bike', 'biking', 'ride', 'road_cycling', 'roadcycling',
        'e_mountain_biking', 'e_biking', 'e_bike'
    ) AND raw_sub_type <> '' AND raw_sub_type <> 'generic' THEN
        sub_type := raw_sub_type;
    END IF;

    IF sub_type = 'generic' THEN
        sub_type := '';
    END IF;

    IF sport_type = 'running' AND sub_type = 'treadmill' THEN
        is_indoor := true;
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
        ('water_tubing'), ('wheelchair_push_run'), ('wheelchair_push_walk'),
        ('windsurfing'), ('yoga')
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
        ('training','hiit'), ('training','strength_training'), ('training','yoga'),
        ('transition','bike_to_run_transition'), ('transition','run_to_bike_transition'), ('transition','swim_to_bike_transition'),
        ('walking','casual_walking'), ('walking','indoor_walking'), ('walking','speed_walking'),
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

CREATE OR REPLACE FUNCTION fn_validate_modifier_filters(f JSONB)
RETURNS BOOLEAN
LANGUAGE sql
IMMUTABLE
AS $$
SELECT
    COALESCE(jsonb_typeof(f) = 'object', false)
    AND NOT EXISTS (
        SELECT 1
        FROM jsonb_each(CASE WHEN jsonb_typeof(f) = 'object' THEN f ELSE '{}'::jsonb END) AS kv(key, value)
        WHERE kv.key NOT IN ('is_electric', 'is_indoor', 'is_virtual', 'is_commute')
           OR jsonb_typeof(kv.value) <> 'boolean'
    );
$$;

CREATE OR REPLACE FUNCTION fp_predicates_from_sport_types(sports TEXT[])
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

CREATE OR REPLACE FUNCTION fp_modifier_filters_from_sport_types(sports TEXT[])
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    raw_sport TEXT;
    n RECORD;
    out JSONB := '{}'::jsonb;
BEGIN
    FOREACH raw_sport IN ARRAY COALESCE(sports, ARRAY[]::text[]) LOOP
        SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
        IF n.is_electric THEN
            out := out || jsonb_build_object('is_electric', true);
        END IF;
        IF n.is_indoor THEN
            out := out || jsonb_build_object('is_indoor', true);
        END IF;
    END LOOP;
    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_default_predicates(base_profile TEXT)
RETURNS JSONB
LANGUAGE sql
IMMUTABLE
AS $$
SELECT CASE base_profile
    WHEN 'hiking' THEN
        '[{"sport":"hiking"},{"sport":"running","sub_type":"trail"},{"sport":"running"},{"sport":"walking"},{"sport":"mountaineering"}]'::jsonb
    WHEN 'mtb' THEN
        '[{"sport":"cycling","sub_type":"mountain"}]'::jsonb
    WHEN 'gravel' THEN
        '[{"sport":"cycling","sub_type":"gravel_cycling"}]'::jsonb
    WHEN 'trekking' THEN
        '[{"sport":"cycling","sub_type":""},{"sport":"cycling","sub_type":"road"},{"sport":"cycling","sub_type":"mixed_surface"}]'::jsonb
    WHEN 'road' THEN
        '[{"sport":"cycling","sub_type":""},{"sport":"cycling","sub_type":"road"},{"sport":"cycling","sub_type":"gravel_cycling"}]'::jsonb
    ELSE '[]'::jsonb
END;
$$;

CREATE OR REPLACE FUNCTION fp_rewrite_conditions(conditions JSONB, surface TEXT, reject_legacy_in_or BOOLEAN)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    cond JSONB;
    out JSONB := '[]'::jsonb;
    op TEXT;
    raw_sport TEXT;
    raw_item JSONB;
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
        IF op NOT IN ('eq', 'neq', 'in', 'not_in') THEN
            RAISE EXCEPTION '%: unsupported sport_type operator "%" for sport taxonomy rewrite', surface, cond->>'op';
        END IF;

        IF op = 'neq' THEN
            raw_sport := cond->>'value';
            IF raw_sport IS NULL OR raw_sport = '' THEN
                RAISE EXCEPTION '%: sport_type condition value must be a string', surface;
            END IF;

            SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
            needs_tuple := raw_sport IS DISTINCT FROM n.sport_type OR n.sub_type <> '' OR n.is_indoor OR n.is_electric;
            IF needs_tuple THEN
                RAISE EXCEPTION '%: unsupported sport_type operator "%" for legacy or subtype sport predicate "%"', surface, cond->>'op', raw_sport;
            END IF;

            out := out || jsonb_build_array(cond);
            CONTINUE;
        END IF;

        IF op IN ('in', 'not_in') THEN
            IF jsonb_typeof(cond->'value') IS DISTINCT FROM 'array' THEN
                RAISE EXCEPTION '%: sport_type % condition value must be a string array', surface, op;
            END IF;

            FOR raw_item IN SELECT value FROM jsonb_array_elements(cond->'value') LOOP
                IF jsonb_typeof(raw_item) <> 'string' THEN
                    RAISE EXCEPTION '%: sport_type % condition value must be a string array', surface, op;
                END IF;
                raw_sport := raw_item #>> '{}';
                SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
                needs_tuple := raw_sport IS DISTINCT FROM n.sport_type OR n.sub_type <> '' OR n.is_indoor OR n.is_electric;
                IF needs_tuple THEN
                    RAISE EXCEPTION '%: unsupported sport_type operator "%" for legacy or subtype sport predicate "%"', surface, cond->>'op', raw_sport;
                END IF;
            END LOOP;

            out := out || jsonb_build_array(cond);
            CONTINUE;
        END IF;

        raw_sport := cond->>'value';
        IF raw_sport IS NULL OR raw_sport = '' THEN
            RAISE EXCEPTION '%: sport_type condition value must be a string', surface;
        END IF;

        SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, '');
        needs_tuple := raw_sport IS DISTINCT FROM n.sport_type OR n.sub_type <> '' OR n.is_indoor OR n.is_electric;
        IF reject_legacy_in_or AND needs_tuple THEN
            RAISE EXCEPTION '%: or group contains legacy sport predicate "%"', surface, raw_sport;
        END IF;

        out := out || jsonb_build_array(jsonb_set(cond, '{value}', to_jsonb(n.sport_type), true));
        IF n.sub_type <> '' THEN
            out := out || jsonb_build_array(jsonb_build_object('field', 'sub_type', 'op', 'eq', 'value', n.sub_type));
        END IF;
        IF n.is_electric THEN
            out := out || jsonb_build_array(jsonb_build_object('field', 'is_electric', 'op', 'eq', 'value', true));
        END IF;
        IF n.is_indoor THEN
            out := out || jsonb_build_array(jsonb_build_object('field', 'is_indoor', 'op', 'eq', 'value', true));
        END IF;
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_rewrite_condition_groups(groups JSONB, surface TEXT)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    group_item JSONB;
    out JSONB := '[]'::jsonb;
    logic TEXT;
    rewritten JSONB;
BEGIN
    IF groups IS NULL OR jsonb_typeof(groups) <> 'array' THEN
        RETURN groups;
    END IF;

    FOR group_item IN SELECT value FROM jsonb_array_elements(groups) LOOP
        logic := lower(btrim(COALESCE(group_item->>'logic', 'and')));
        rewritten := fp_rewrite_conditions(COALESCE(group_item->'conditions', '[]'::jsonb), surface, logic = 'or');
        out := out || jsonb_build_array(jsonb_set(group_item, '{conditions}', rewritten, true));
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_rewrite_actions(actions JSONB, surface TEXT)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    action_item JSONB;
    out JSONB := '[]'::jsonb;
    action_name TEXT;
    params JSONB;
    raw_sport TEXT;
    raw_sub_type TEXT;
    n RECORD;
    new_params JSONB;
    new_action JSONB;
    modifier_field TEXT;
    modifier_value BOOLEAN;
    modifier_writes JSONB := '{}'::jsonb;
BEGIN
    IF actions IS NULL OR jsonb_typeof(actions) <> 'array' THEN
        RETURN actions;
    END IF;

    FOR action_item IN SELECT value FROM jsonb_array_elements(actions) LOOP
        action_name := COALESCE(action_item->>'action', action_item->>'type');
        IF action_name = 'set_field' AND action_item->'params'->>'field' = 'sport_type' THEN
            raw_sport := action_item->'params'->>'value';
            IF raw_sport IS NULL OR raw_sport = '' THEN
                RAISE EXCEPTION '%: set_field sport_type value must be a string', surface;
            END IF;
            action_item := jsonb_build_object(
                'action', 'set_sport_type',
                'params', jsonb_build_object('sport_type', raw_sport)
            );
            action_name := 'set_sport_type';
        END IF;

        IF action_name IS DISTINCT FROM 'set_sport_type' THEN
            modifier_field := NULL;
            modifier_value := NULL;
            IF action_name = 'set_electric' THEN
                modifier_field := 'is_electric';
                modifier_value := COALESCE((action_item->'params'->>'electric')::boolean, true);
            ELSIF action_name = 'set_indoor' THEN
                modifier_field := 'is_indoor';
                modifier_value := COALESCE((action_item->'params'->>'indoor')::boolean, true);
            ELSIF action_name = 'set_virtual' THEN
                modifier_field := 'is_virtual';
                modifier_value := COALESCE((action_item->'params'->>'virtual')::boolean, true);
            ELSIF action_name = 'set_commute' THEN
                modifier_field := 'is_commute';
                modifier_value := COALESCE((action_item->'params'->>'commute')::boolean, true);
            ELSIF action_name = 'set_field' AND action_item->'params'->>'field' IN ('is_electric', 'is_indoor', 'is_virtual', 'is_commute') THEN
                modifier_field := action_item->'params'->>'field';
                modifier_value := (action_item->'params'->>'value')::boolean;
            END IF;
            IF modifier_field IS NOT NULL THEN
                IF modifier_writes ? modifier_field THEN
                    IF (modifier_writes->>modifier_field)::boolean IS DISTINCT FROM modifier_value THEN
                        RAISE EXCEPTION '%: conflicting writes to %', surface, modifier_field;
                    END IF;
                ELSE
                    modifier_writes := modifier_writes || jsonb_build_object(modifier_field, modifier_value);
                END IF;
            END IF;
            out := out || jsonb_build_array(action_item);
            CONTINUE;
        END IF;

        params := action_item->'params';
        IF params IS NULL OR jsonb_typeof(params) <> 'object' OR NOT (params ? 'sport_type') THEN
            out := out || jsonb_build_array(action_item);
            CONTINUE;
        END IF;

        raw_sport := params->>'sport_type';
        raw_sub_type := COALESCE(params->>'sub_type', '');
        SELECT * INTO n FROM fp_sport_taxonomy_normalize(raw_sport, raw_sub_type);
        new_params := (params || jsonb_build_object('sport_type', n.sport_type)) - 'sub_type';
        IF n.sub_type <> '' THEN
            new_params := new_params || jsonb_build_object('sub_type', n.sub_type);
        END IF;
        new_action := (action_item - 'type') || jsonb_build_object('action', 'set_sport_type', 'params', new_params);
        out := out || jsonb_build_array(new_action);

        IF n.is_electric THEN
            IF modifier_writes ? 'is_electric' THEN
                IF (modifier_writes->>'is_electric')::boolean IS DISTINCT FROM true THEN
                    RAISE EXCEPTION '%: conflicting writes to is_electric', surface;
                END IF;
            ELSE
                modifier_writes := modifier_writes || jsonb_build_object('is_electric', true);
                out := out || jsonb_build_array(jsonb_build_object('action', 'set_electric', 'params', jsonb_build_object('electric', true)));
            END IF;
        END IF;
        IF n.is_indoor THEN
            IF modifier_writes ? 'is_indoor' THEN
                IF (modifier_writes->>'is_indoor')::boolean IS DISTINCT FROM true THEN
                    RAISE EXCEPTION '%: conflicting writes to is_indoor', surface;
                END IF;
            ELSE
                modifier_writes := modifier_writes || jsonb_build_object('is_indoor', true);
                out := out || jsonb_build_array(jsonb_build_object('action', 'set_indoor', 'params', jsonb_build_object('indoor', true)));
            END IF;
        END IF;
    END LOOP;

    RETURN out;
END;
$$;

CREATE OR REPLACE FUNCTION fp_rewrite_branches(branches JSONB)
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
            branch_item := jsonb_set(
                branch_item,
                '{condition_groups}',
                fp_rewrite_condition_groups(branch_item->'condition_groups', 'branches[].condition_groups'),
                true
            );
        END IF;
        IF branch_item ? 'actions' THEN
            branch_item := jsonb_set(
                branch_item,
                '{actions}',
                fp_rewrite_actions(branch_item->'actions', 'branches[].actions'),
                true
            );
        END IF;
        out := out || jsonb_build_array(branch_item);
    END LOOP;

    RETURN out;
END;
$$;

ALTER TABLE routing_profiles
    ADD COLUMN IF NOT EXISTS predicates JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS modifier_filters JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE planned_routes
    ADD COLUMN IF NOT EXISTS sub_type TEXT,
    ADD COLUMN IF NOT EXISTS is_electric BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_indoor BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_virtual BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_commute BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE recording_groups
    ADD COLUMN IF NOT EXISTS sub_type TEXT,
    ADD COLUMN IF NOT EXISTS is_electric BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_indoor BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS is_virtual BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS segments_legacy_sub_type_hints (
    segment_id UUID PRIMARY KEY REFERENCES segments(id) ON DELETE CASCADE,
    original_sport_types TEXT[] NOT NULL,
    original_sub_types TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
DECLARE
    affected_segments INTEGER;
    affected_with_subtypes INTEGER;
BEGIN
    SELECT COUNT(DISTINCT seg.id)
    INTO affected_segments
    FROM segments seg
    CROSS JOIN LATERAL unnest(seg.allowed_sport_types) AS raw(sport_type)
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(raw.sport_type, '') AS n
    WHERE raw.sport_type IN (
        'mountain_biking', 'gravel_cycling', 'indoor_cycling',
        'trail_running', 'treadmill_running',
        'lap_swimming', 'open_water_swimming',
        'bike', 'biking', 'ride', 'road_cycling', 'roadcycling',
        'e_mountain_biking', 'e_biking', 'e_bike'
    )
      AND NOT n.sport_type = ANY(seg.allowed_sport_types);

    SELECT COUNT(*)
    INTO affected_with_subtypes
    FROM segments
    WHERE allowed_sport_types && ARRAY[
        'mountain_biking', 'gravel_cycling', 'indoor_cycling',
        'trail_running', 'treadmill_running',
        'lap_swimming', 'open_water_swimming',
        'bike', 'biking', 'ride', 'road_cycling', 'roadcycling',
        'e_mountain_biking', 'e_biking', 'e_bike'
    ]::text[]
      AND cardinality(COALESCE(allowed_sub_types, ARRAY[]::text[])) > 0;

    IF affected_with_subtypes > 0 THEN
        RAISE EXCEPTION 'segment already has subtype restrictions; migrate through Wave predicates before collapsing top sports';
    END IF;

    IF affected_segments > 0
       AND COALESCE(current_setting('skadia.allow_segment_sport_broadening', true), '') <> 'on' THEN
        RAISE EXCEPTION 'segment sport collapse would broaden segment matching for % existing segments', affected_segments;
    END IF;
END;
$$;

INSERT INTO segments_legacy_sub_type_hints (segment_id, original_sport_types, original_sub_types)
SELECT id, allowed_sport_types, allowed_sub_types
FROM segments
WHERE allowed_sport_types && ARRAY[
    'mountain_biking', 'gravel_cycling', 'indoor_cycling',
    'trail_running', 'treadmill_running',
    'lap_swimming', 'open_water_swimming',
    'bike', 'biking', 'ride', 'road_cycling', 'roadcycling',
    'e_mountain_biking', 'e_biking', 'e_bike'
]::text[]
ON CONFLICT (segment_id) DO NOTHING;

WITH normalized_activities AS (
    SELECT
        a.id,
        a.sport_type AS old_sport_type,
        COALESCE(a.sub_type, '') AS old_sub_type,
        a.is_indoor AS old_is_indoor,
        a.is_electric AS old_is_electric,
        n.sport_type,
        n.sub_type,
        n.is_indoor,
        n.is_electric
    FROM activities a
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(a.sport_type, COALESCE(a.sub_type, '')) AS n
)
UPDATE activities a
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, ''),
    is_indoor = a.is_indoor OR n.is_indoor,
    is_electric = a.is_electric OR n.is_electric
FROM normalized_activities n
WHERE a.id = n.id
  AND (
      n.old_sport_type IS DISTINCT FROM n.sport_type
      OR n.old_sub_type IS DISTINCT FROM n.sub_type
      OR (n.is_indoor AND n.old_is_indoor = false)
      OR (n.is_electric AND n.old_is_electric = false)
  );

WITH normalized_parsed_activities AS (
    SELECT
        a.id,
        a.parsed_sport_type AS old_sport_type,
        COALESCE(a.parsed_sub_type, '') AS old_sub_type,
        a.is_indoor AS old_is_indoor,
        a.is_electric AS old_is_electric,
        n.sport_type,
        n.sub_type,
        n.is_indoor,
        n.is_electric
    FROM activities a
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(a.parsed_sport_type, COALESCE(a.parsed_sub_type, '')) AS n
    WHERE a.parsed_sport_type IS NOT NULL
)
UPDATE activities a
SET parsed_sport_type = n.sport_type,
    parsed_sub_type = NULLIF(n.sub_type, ''),
    is_indoor = a.is_indoor OR n.is_indoor,
    is_electric = a.is_electric OR n.is_electric
FROM normalized_parsed_activities n
WHERE a.id = n.id
  AND (
      n.old_sport_type IS DISTINCT FROM n.sport_type
      OR n.old_sub_type IS DISTINCT FROM n.sub_type
      OR (n.is_indoor AND a.is_indoor = false)
      OR (n.is_electric AND a.is_electric = false)
  );

WITH normalized_heatmap_lines AS (
    SELECT
        h.id,
        h.sport_type AS old_sport_type,
        COALESCE(h.sub_type, '') AS old_sub_type,
        n.sport_type,
        n.sub_type
    FROM heatmap_lines h
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(h.sport_type, COALESCE(h.sub_type, '')) AS n
    WHERE h.sport_type IS NOT NULL
)
UPDATE heatmap_lines h
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM normalized_heatmap_lines n
WHERE h.id = n.id
  AND (n.old_sport_type IS DISTINCT FROM n.sport_type OR n.old_sub_type IS DISTINCT FROM n.sub_type);

WITH normalized_heatmap_points AS (
    SELECT
        h.id,
        h.sport_type AS old_sport_type,
        COALESCE(h.sub_type, '') AS old_sub_type,
        n.sport_type,
        n.sub_type
    FROM heatmap_points h
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(h.sport_type, COALESCE(h.sub_type, '')) AS n
    WHERE h.sport_type IS NOT NULL
)
UPDATE heatmap_points h
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, '')
FROM normalized_heatmap_points n
WHERE h.id = n.id
  AND (n.old_sport_type IS DISTINCT FROM n.sport_type OR n.old_sub_type IS DISTINCT FROM n.sub_type);

WITH normalized_segment_efforts AS (
    SELECT
        se.ctid AS row_id,
        se.sport_type AS old_sport_type,
        se.sub_type AS old_sub_type,
        n.sport_type,
        COALESCE(NULLIF(n.sub_type, ''), 'generic') AS sub_type
    FROM segment_efforts se
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(se.sport_type, COALESCE(se.sub_type, '')) AS n
)
UPDATE segment_efforts se
SET sport_type = n.sport_type,
    sub_type = n.sub_type
FROM normalized_segment_efforts n
WHERE se.ctid = n.row_id
  AND (n.old_sport_type IS DISTINCT FROM n.sport_type OR n.old_sub_type IS DISTINCT FROM n.sub_type);

UPDATE segments s
SET allowed_sport_types = normalized.sports
FROM (
    SELECT
        seg.id,
        array_agg(DISTINCT n.sport_type ORDER BY n.sport_type) AS sports
    FROM segments seg
    CROSS JOIN LATERAL unnest(seg.allowed_sport_types) AS raw(sport_type)
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(raw.sport_type, '') AS n
    WHERE seg.allowed_sport_types IS NOT NULL
    GROUP BY seg.id
) AS normalized
WHERE s.id = normalized.id;

WITH normalized_planned_routes AS (
    SELECT
        pr.id,
        pr.sport_type AS old_sport_type,
        COALESCE(pr.sub_type, '') AS old_sub_type,
        pr.is_electric AS old_is_electric,
        pr.is_indoor AS old_is_indoor,
        n.sport_type,
        n.sub_type,
        n.is_electric,
        n.is_indoor
    FROM planned_routes pr
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(pr.sport_type, COALESCE(pr.sub_type, '')) AS n
)
UPDATE planned_routes pr
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, ''),
    is_electric = pr.is_electric OR n.is_electric,
    is_indoor = pr.is_indoor OR n.is_indoor
FROM normalized_planned_routes n
WHERE pr.id = n.id
  AND (n.old_sport_type IS DISTINCT FROM n.sport_type
       OR n.old_sub_type IS DISTINCT FROM n.sub_type
       OR (n.is_electric AND n.old_is_electric = false)
       OR (n.is_indoor AND n.old_is_indoor = false));

WITH normalized_recording_groups AS (
    SELECT
        rg.id,
        rg.sport_type AS old_sport_type,
        COALESCE(rg.sub_type, '') AS old_sub_type,
        rg.is_indoor AS old_is_indoor,
        rg.is_electric AS old_is_electric,
        n.sport_type,
        n.sub_type,
        n.is_indoor,
        n.is_electric
    FROM recording_groups rg
    CROSS JOIN LATERAL fp_sport_taxonomy_normalize(rg.sport_type, COALESCE(rg.sub_type, '')) AS n
    WHERE rg.sport_type IS NOT NULL
)
UPDATE recording_groups rg
SET sport_type = n.sport_type,
    sub_type = NULLIF(n.sub_type, ''),
    is_indoor = rg.is_indoor OR n.is_indoor,
    is_electric = rg.is_electric OR n.is_electric
FROM normalized_recording_groups n
WHERE rg.id = n.id
  AND (n.old_sport_type IS DISTINCT FROM n.sport_type
       OR n.old_sub_type IS DISTINCT FROM n.sub_type
       OR (n.is_indoor AND n.old_is_indoor = false)
       OR (n.is_electric AND n.old_is_electric = false));

UPDATE routing_profiles rp
SET predicates = CASE
        WHEN COALESCE(array_length(rp.sport_types, 1), 0) > 0 THEN fp_predicates_from_sport_types(rp.sport_types)
        ELSE fp_default_predicates(rp.base_profile)
    END,
    modifier_filters = fp_modifier_filters_from_sport_types(rp.sport_types),
    sport_types = (
        SELECT COALESCE(array_agg(DISTINCT pred->>'sport' ORDER BY pred->>'sport'), ARRAY[]::text[])
        FROM jsonb_array_elements(
            CASE
                WHEN COALESCE(array_length(rp.sport_types, 1), 0) > 0 THEN fp_predicates_from_sport_types(rp.sport_types)
                ELSE fp_default_predicates(rp.base_profile)
            END
        ) AS pred
    );

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM routing_profiles WHERE jsonb_array_length(predicates) = 0) THEN
        RAISE EXCEPTION 'routing_profiles contains empty sport_types without a deterministic base_profile fallback';
    END IF;
END;
$$;

UPDATE automation_rules
SET conditions = fp_rewrite_conditions(conditions, 'conditions', false),
    condition_groups = fp_rewrite_condition_groups(condition_groups, 'condition_groups'),
    actions = fp_rewrite_actions(actions, 'actions'),
    branches = fp_rewrite_branches(branches);

ALTER TABLE routing_profiles
    DROP CONSTRAINT IF EXISTS routing_profiles_predicates_valid,
    ADD CONSTRAINT routing_profiles_predicates_valid
        CHECK (jsonb_array_length(predicates) > 0 AND fn_validate_predicates(predicates));

ALTER TABLE routing_profiles
    DROP CONSTRAINT IF EXISTS routing_profiles_modifier_filters_valid,
    ADD CONSTRAINT routing_profiles_modifier_filters_valid
        CHECK (fn_validate_modifier_filters(modifier_filters));

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

DROP FUNCTION IF EXISTS fp_rewrite_branches(JSONB);
DROP FUNCTION IF EXISTS fp_rewrite_actions(JSONB, TEXT);
DROP FUNCTION IF EXISTS fp_rewrite_condition_groups(JSONB, TEXT);
DROP FUNCTION IF EXISTS fp_rewrite_conditions(JSONB, TEXT, BOOLEAN);
DROP FUNCTION IF EXISTS fp_default_predicates(TEXT);
DROP FUNCTION IF EXISTS fp_modifier_filters_from_sport_types(TEXT[]);
DROP FUNCTION IF EXISTS fp_predicates_from_sport_types(TEXT[]);
