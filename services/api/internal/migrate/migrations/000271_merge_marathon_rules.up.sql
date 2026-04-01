-- Merge 3 separate Marathon/Halbmarathon/Ultramarathon rules into one ELSE-IF rule per user
DO $$
DECLARE
    u RECORD;
    ultra_tag_id TEXT;
    mara_tag_id TEXT;
    halb_tag_id TEXT;
BEGIN
    FOR u IN SELECT DISTINCT user_id FROM automation_rules
             WHERE name IN ('Ultramarathon', 'Marathon', 'Halbmarathon') AND is_system = false
    LOOP
        -- Extract tag_ids from the existing rules' actions
        SELECT (actions->0->'params'->>'tag_id') INTO ultra_tag_id
        FROM automation_rules WHERE user_id = u.user_id AND name = 'Ultramarathon' AND is_system = false LIMIT 1;

        SELECT (actions->0->'params'->>'tag_id') INTO mara_tag_id
        FROM automation_rules WHERE user_id = u.user_id AND name = 'Marathon' AND is_system = false LIMIT 1;

        SELECT (actions->0->'params'->>'tag_id') INTO halb_tag_id
        FROM automation_rules WHERE user_id = u.user_id AND name = 'Halbmarathon' AND is_system = false LIMIT 1;

        -- Only merge if all 3 exist
        IF ultra_tag_id IS NOT NULL AND mara_tag_id IS NOT NULL AND halb_tag_id IS NOT NULL THEN
            -- Delete the 3 separate rules
            DELETE FROM automation_rules
            WHERE user_id = u.user_id AND name IN ('Ultramarathon', 'Marathon', 'Halbmarathon') AND is_system = false;

            -- Create one combined ELSE-IF rule
            INSERT INTO automation_rules
                (user_id, name, is_active, priority, conditions, conditions_logic, actions,
                 branches, trigger_event, stop_on_match, apply_retroactively)
            VALUES (
                u.user_id, 'Laufdistanz', true, 10, '[]'::jsonb, 'and', '[]'::jsonb,
                json_build_array(
                    json_build_object(
                        'type', 'if',
                        'condition_groups', json_build_array(json_build_object(
                            'logic', 'and',
                            'conditions', json_build_array(
                                json_build_object('field', 'sport_type', 'op', 'eq', 'value', 'running'),
                                json_build_object('field', 'distance_m', 'op', 'gte', 'value', 42196)
                            )
                        )),
                        'actions', json_build_array(json_build_object('action', 'add_tag', 'params', json_build_object('tag_id', ultra_tag_id)))
                    ),
                    json_build_object(
                        'type', 'else_if',
                        'condition_groups', json_build_array(json_build_object(
                            'logic', 'and',
                            'conditions', json_build_array(
                                json_build_object('field', 'distance_m', 'op', 'gte', 'value', 42000)
                            )
                        )),
                        'actions', json_build_array(json_build_object('action', 'add_tag', 'params', json_build_object('tag_id', mara_tag_id)))
                    ),
                    json_build_object(
                        'type', 'else_if',
                        'condition_groups', json_build_array(json_build_object(
                            'logic', 'and',
                            'conditions', json_build_array(
                                json_build_object('field', 'distance_m', 'op', 'gte', 'value', 21000)
                            )
                        )),
                        'actions', json_build_array(json_build_object('action', 'add_tag', 'params', json_build_object('tag_id', halb_tag_id)))
                    )
                )::jsonb,
                'on_import', false, true
            );
        END IF;
    END LOOP;
END $$;
