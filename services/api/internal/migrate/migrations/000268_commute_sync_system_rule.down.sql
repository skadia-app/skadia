-- 000268_commute_sync_system_rule.down.sql

-- 1. Remove commute_sync system rules
DELETE FROM automation_rules WHERE system_type = 'commute_sync';

-- 2. Restore system_type CHECK constraint without commute_sync
ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
    CHECK (system_type IN ('electric_sync', 'round_trip', 'frost', 'nacht', 'virtual_sync', 'indoor_sync'));

-- 3. Restore trigger without is_commute/Pendeln (as in 000261)
CREATE OR REPLACE FUNCTION sync_system_flags_and_tags() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    -- Direction A: tags TEXT[] changed → derive boolean flags
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_indoor := 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_electric := 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
    END IF;

    -- Direction B: is_indoor changed directly → sync tag + junction
    IF NEW.is_indoor IS DISTINCT FROM OLD.is_indoor THEN
        IF NEW.is_indoor AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
                    VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT DO NOTHING;
                UPDATE tags SET activity_count = activity_count + 1 WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_indoor AND 'Indoor' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Indoor');
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
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
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
                    VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT DO NOTHING;
                UPDATE tags SET activity_count = activity_count + 1 WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_electric AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            SELECT id INTO v_tag_id FROM tags
                WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = GREATEST(activity_count - 1, 0) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags ON activities;
CREATE TRIGGER trg_sync_system_flags_and_tags
    BEFORE UPDATE OF tags, is_indoor, is_electric ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_flags_and_tags();
