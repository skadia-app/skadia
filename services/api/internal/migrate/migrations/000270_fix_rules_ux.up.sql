-- Fix: Migration 000261 vergaß is_system im INSERT → Indoor-Sync hat is_system=false
UPDATE automation_rules SET is_system = true WHERE system_type = 'indoor_sync' AND is_system = false;

-- Rename system rules to user-friendly names
UPDATE automation_rules SET name = 'Indoor' WHERE system_type = 'indoor_sync';
UPDATE automation_rules SET name = 'Pendeln' WHERE system_type = 'commute_sync';
UPDATE automation_rules SET name = 'Virtuell' WHERE system_type = 'virtual_sync';
-- E-Bike bleibt (hat keinen Suffix), Rundtour/Frost/Nacht bleiben auch

-- Seed-Tags und Seed-Rules für alle bestehenden User
DO $$
DECLARE
    u RECORD;
    tag_id UUID;
BEGIN
    FOR u IN SELECT id FROM users LOOP
        -- Für jeden User: Tags anlegen wenn nicht vorhanden
        FOR tag_id IN
            INSERT INTO tags (user_id, name, color)
            VALUES
                (u.id, 'Ultramarathon', '#7C3AED'),
                (u.id, 'Marathon', '#D97706'),
                (u.id, 'Halbmarathon', '#9CA3AF'),
                (u.id, 'Intervall-Training', '#EA580C'),
                (u.id, 'Mit Einkehr', '#16A34A'),
                (u.id, 'Grenzquerung', '#0891B2')
            ON CONFLICT (user_id, name) DO UPDATE SET name = EXCLUDED.name
            RETURNING id
        LOOP
            -- Tags created
        END LOOP;

        -- Ultramarathon Rule: distance >= 42196 AND sport_type = running
        INSERT INTO automation_rules (user_id, name, is_active, priority, conditions, conditions_logic, actions, trigger_event, stop_on_match, apply_retroactively)
        SELECT u.id, 'Ultramarathon', true, 10,
            json_build_array(json_build_object('field','distance_m','op','gte','value',42196), json_build_object('field','sport_type','op','eq','value','running'))::jsonb,
            'and',
            json_build_array(json_build_object('action','add_tag','params',json_build_object('tag_id', t.id::text)))::jsonb,
            'on_import', false, true
        FROM tags t WHERE t.user_id = u.id AND t.name = 'Ultramarathon'
        AND NOT EXISTS (SELECT 1 FROM automation_rules WHERE user_id = u.id AND name = 'Ultramarathon' AND is_system = false);

        -- Marathon Rule: distance between 42000 and 42195 AND sport_type = running
        INSERT INTO automation_rules (user_id, name, is_active, priority, conditions, conditions_logic, actions, trigger_event, stop_on_match, apply_retroactively)
        SELECT u.id, 'Marathon', true, 20,
            json_build_array(json_build_object('field','distance_m','op','between','value','42000,42195'), json_build_object('field','sport_type','op','eq','value','running'))::jsonb,
            'and',
            json_build_array(json_build_object('action','add_tag','params',json_build_object('tag_id', t.id::text)))::jsonb,
            'on_import', false, true
        FROM tags t WHERE t.user_id = u.id AND t.name = 'Marathon'
        AND NOT EXISTS (SELECT 1 FROM automation_rules WHERE user_id = u.id AND name = 'Marathon' AND is_system = false);

        -- Halbmarathon Rule: distance between 21000 and 41999 AND sport_type = running
        INSERT INTO automation_rules (user_id, name, is_active, priority, conditions, conditions_logic, actions, trigger_event, stop_on_match, apply_retroactively)
        SELECT u.id, 'Halbmarathon', true, 30,
            json_build_array(json_build_object('field','distance_m','op','between','value','21000,41999'), json_build_object('field','sport_type','op','eq','value','running'))::jsonb,
            'and',
            json_build_array(json_build_object('action','add_tag','params',json_build_object('tag_id', t.id::text)))::jsonb,
            'on_import', false, true
        FROM tags t WHERE t.user_id = u.id AND t.name = 'Halbmarathon'
        AND NOT EXISTS (SELECT 1 FROM automation_rules WHERE user_id = u.id AND name = 'Halbmarathon' AND is_system = false);

        -- Intervall-Training: has_structured_intervals = true
        INSERT INTO automation_rules (user_id, name, is_active, priority, conditions, conditions_logic, actions, trigger_event, stop_on_match, apply_retroactively)
        SELECT u.id, 'Intervall-Training', true, 40,
            json_build_array(json_build_object('field','has_structured_intervals','op','eq','value',true))::jsonb,
            'and',
            json_build_array(json_build_object('action','add_tag','params',json_build_object('tag_id', t.id::text)))::jsonb,
            'on_enrichment_complete', false, true
        FROM tags t WHERE t.user_id = u.id AND t.name = 'Intervall-Training'
        AND NOT EXISTS (SELECT 1 FROM automation_rules WHERE user_id = u.id AND name = 'Intervall-Training' AND is_system = false);

        -- Mit Einkehr: has_gastronomy_stop = true
        INSERT INTO automation_rules (user_id, name, is_active, priority, conditions, conditions_logic, actions, trigger_event, stop_on_match, apply_retroactively)
        SELECT u.id, 'Mit Einkehr', true, 50,
            json_build_array(json_build_object('field','has_gastronomy_stop','op','eq','value',true))::jsonb,
            'and',
            json_build_array(json_build_object('action','add_tag','params',json_build_object('tag_id', t.id::text)))::jsonb,
            'on_enrichment_complete', false, true
        FROM tags t WHERE t.user_id = u.id AND t.name = 'Mit Einkehr'
        AND NOT EXISTS (SELECT 1 FROM automation_rules WHERE user_id = u.id AND name = 'Mit Einkehr' AND is_system = false);

        -- Grenzquerung: border_crossing_count > 0
        INSERT INTO automation_rules (user_id, name, is_active, priority, conditions, conditions_logic, actions, trigger_event, stop_on_match, apply_retroactively)
        SELECT u.id, 'Grenzquerung', true, 60,
            json_build_array(json_build_object('field','border_crossing_count','op','gt','value',0))::jsonb,
            'and',
            json_build_array(json_build_object('action','add_tag','params',json_build_object('tag_id', t.id::text)))::jsonb,
            'on_enrichment_complete', false, true
        FROM tags t WHERE t.user_id = u.id AND t.name = 'Grenzquerung'
        AND NOT EXISTS (SELECT 1 FROM automation_rules WHERE user_id = u.id AND name = 'Grenzquerung' AND is_system = false);
    END LOOP;
END $$;
