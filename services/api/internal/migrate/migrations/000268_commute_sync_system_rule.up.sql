-- 000268_commute_sync_system_rule.up.sql

-- 1. Update system_type CHECK constraint
ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
    CHECK (system_type IN ('electric_sync', 'round_trip', 'frost', 'nacht', 'virtual_sync', 'indoor_sync', 'commute_sync'));

-- 2. Create system tag "Pendeln" for all users (promote existing or insert new)
-- Uses ON CONFLICT to handle users who already have a non-system "Pendeln" tag.
INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
SELECT u.id, 'Pendeln', '#6B7280', '#FFFFFF', true, false,
       'Automatisch bei Pendel-Aktivitäten (Sync mit is_commute)'
FROM users u
ON CONFLICT (user_id, name) DO UPDATE
    SET is_system = true,
        description = EXCLUDED.description;

-- 3. Backfill: assign Pendeln tag to existing is_commute=true activities
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'migration'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'Pendeln' AND t.is_system = true
WHERE a.is_commute = true
ON CONFLICT DO NOTHING;

UPDATE tags SET activity_count = (
    SELECT COUNT(*) FROM activity_tags at2 WHERE at2.tag_id = tags.id
) WHERE name = 'Pendeln' AND is_system = true;

UPDATE activities SET tags = array_append(tags, 'Pendeln')
WHERE is_commute = true AND NOT ('Pendeln' = ANY(COALESCE(tags, '{}')));

-- 4. Extend bidirectional trigger to include Pendeln <-> is_commute
CREATE OR REPLACE FUNCTION sync_system_flags_and_tags() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    -- Direction A: tags TEXT[] changed → derive boolean flags
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_indoor := 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_electric := 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_commute := 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));
    END IF;

    -- Direction B: is_indoor changed directly → sync tag + junction
    IF NEW.is_indoor IS DISTINCT FROM OLD.is_indoor THEN
        IF NEW.is_indoor AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT DO NOTHING;
                UPDATE tags SET activity_count = activity_count + 1 WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_indoor AND 'Indoor' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Indoor');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = GREATEST(activity_count - 1, 0) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    -- Direction C: is_electric changed directly → sync tag + junction
    IF NEW.is_electric IS DISTINCT FROM OLD.is_electric THEN
        IF NEW.is_electric AND NOT ('E-Bike' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'E-Bike');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT DO NOTHING;
                UPDATE tags SET activity_count = activity_count + 1 WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_electric AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = GREATEST(activity_count - 1, 0) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    -- Direction D: is_commute changed directly → sync Pendeln tag + junction
    IF NEW.is_commute IS DISTINCT FROM OLD.is_commute THEN
        IF NEW.is_commute AND NOT ('Pendeln' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Pendeln');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT DO NOTHING;
                UPDATE tags SET activity_count = activity_count + 1 WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_commute AND 'Pendeln' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Pendeln');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = GREATEST(activity_count - 1, 0) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Update trigger to also fire on is_commute changes
DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags ON activities;
CREATE TRIGGER trg_sync_system_flags_and_tags
    BEFORE UPDATE OF tags, is_indoor, is_electric, is_commute ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_flags_and_tags();

-- 5. Create commute_sync system rule for all users (skipped by rules engine, managed by trigger)
INSERT INTO automation_rules (id, user_id, name, is_active, is_system, system_type, priority, conditions, conditions_logic, actions)
SELECT gen_random_uuid(), u.id, 'Pendeln-Sync', true, true, 'commute_sync', -100, '[]'::jsonb, 'and', '[]'::jsonb
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM automation_rules r WHERE r.user_id = u.id AND r.system_type = 'commute_sync'
);
