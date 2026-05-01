-- Plan T rollback is intentionally conservative and lossy.
-- wheelchair_push/walk_speed and wheelchair_push/run_speed are converted back
-- to the former top-level values. strength_training is not converted back to
-- strength because true strength_training and formerly broad strength rows
-- cannot be distinguished safely.

SELECT pg_advisory_xact_lock(330999, 1);

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
           OR NOT (pred ? 'sport')
           OR jsonb_typeof(pred->'sport') <> 'string'
           OR NOT EXISTS (SELECT 1 FROM allowed_sports s WHERE s.sport = pred->>'sport')
           OR (
                pred ? 'sub_type'
                AND pred->>'sub_type' <> ''
                AND NOT EXISTS (
                    SELECT 1
                    FROM allowed_subtypes st
                    WHERE st.sport = pred->>'sport'
                      AND st.sub_type = pred->>'sub_type'
                )
           )
    );
$$;

CREATE OR REPLACE FUNCTION fp_plan_t_down_predicates(predicates JSONB)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    pred JSONB;
    out JSONB := '[]'::jsonb;
BEGIN
    IF predicates IS NULL OR jsonb_typeof(predicates) <> 'array' THEN
        RETURN predicates;
    END IF;
    FOR pred IN SELECT value FROM jsonb_array_elements(predicates) LOOP
        IF pred->>'sport' = 'wheelchair_push' AND pred->>'sub_type' = 'walk_speed' THEN
            out := out || jsonb_build_array(jsonb_build_object('sport', 'wheelchair_push_walk'));
        ELSIF pred->>'sport' = 'wheelchair_push' AND pred->>'sub_type' = 'run_speed' THEN
            out := out || jsonb_build_array(jsonb_build_object('sport', 'wheelchair_push_run'));
        ELSE
            out := out || jsonb_build_array(pred);
        END IF;
    END LOOP;
    RETURN out;
END;
$$;

UPDATE activities
SET sport_type = 'wheelchair_push_walk',
    sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_walk' ELSE NULL END
WHERE sport_type = 'wheelchair_push' AND sub_type = 'walk_speed';
UPDATE activities
SET sport_type = 'wheelchair_push_run',
    sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_run' ELSE NULL END
WHERE sport_type = 'wheelchair_push' AND sub_type = 'run_speed';
UPDATE activities
SET parsed_sport_type = 'wheelchair_push_walk',
    parsed_sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_walk' ELSE NULL END
WHERE parsed_sport_type = 'wheelchair_push' AND parsed_sub_type = 'walk_speed';
UPDATE activities
SET parsed_sport_type = 'wheelchair_push_run',
    parsed_sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_run' ELSE NULL END
WHERE parsed_sport_type = 'wheelchair_push' AND parsed_sub_type = 'run_speed';

UPDATE heatmap_lines SET sport_type = 'wheelchair_push_walk', sub_type = NULL
WHERE sport_type = 'wheelchair_push' AND sub_type = 'walk_speed';
UPDATE heatmap_lines SET sport_type = 'wheelchair_push_run', sub_type = NULL
WHERE sport_type = 'wheelchair_push' AND sub_type = 'run_speed';
UPDATE heatmap_points SET sport_type = 'wheelchair_push_walk', sub_type = NULL
WHERE sport_type = 'wheelchair_push' AND sub_type = 'walk_speed';
UPDATE heatmap_points SET sport_type = 'wheelchair_push_run', sub_type = NULL
WHERE sport_type = 'wheelchair_push' AND sub_type = 'run_speed';
UPDATE segment_efforts SET sport_type = 'wheelchair_push_walk', sub_type = 'generic'
WHERE sport_type = 'wheelchair_push' AND sub_type = 'walk_speed';
UPDATE segment_efforts SET sport_type = 'wheelchair_push_run', sub_type = 'generic'
WHERE sport_type = 'wheelchair_push' AND sub_type = 'run_speed';

UPDATE planned_routes
SET sport_type = 'wheelchair_push_walk',
    sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_walk' ELSE NULL END
WHERE sport_type = 'wheelchair_push' AND sub_type = 'walk_speed';
UPDATE planned_routes
SET sport_type = 'wheelchair_push_run',
    sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_run' ELSE NULL END
WHERE sport_type = 'wheelchair_push' AND sub_type = 'run_speed';
UPDATE recording_groups
SET sport_type = 'wheelchair_push_walk',
    sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_walk' ELSE NULL END
WHERE sport_type = 'wheelchair_push' AND sub_type = 'walk_speed';
UPDATE recording_groups
SET sport_type = 'wheelchair_push_run',
    sub_type = CASE WHEN is_indoor THEN 'indoor_wheelchair_run' ELSE NULL END
WHERE sport_type = 'wheelchair_push' AND sub_type = 'run_speed';

UPDATE segments
SET allowed_predicates = fp_plan_t_down_predicates(allowed_predicates)
WHERE allowed_predicates IS NOT NULL;
UPDATE routing_profiles
SET predicates = fp_plan_t_down_predicates(predicates);
UPDATE routing_profiles
SET sport_types = (
    SELECT COALESCE(array_agg(DISTINCT pred->>'sport' ORDER BY pred->>'sport'), ARRAY[]::text[])
    FROM jsonb_array_elements(predicates) pred
);

-- Automation-rule rollback is intentionally conservative: split tuple/modifier
-- rules introduced by the up migration remain in their more specific shape.
-- Reconstructing the exact old top-level value from arbitrary JSON rule logic
-- would risk erasing user-authored modifier conditions/actions.

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

DROP FUNCTION IF EXISTS fp_plan_t_down_predicates(JSONB);
