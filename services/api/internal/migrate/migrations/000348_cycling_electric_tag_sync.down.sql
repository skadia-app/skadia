-- 000348_cycling_electric_tag_sync.down.sql

-- restore broad E-Bike tag sync for every is_electric activity.

SELECT pg_advisory_xact_lock(hashtext('cycling_electric_tag_sync'), 1);

UPDATE tags
SET description = 'Automatisch bei E-Bike-Aktivitäten (Sync mit is_electric)'
WHERE name = 'E-Bike' AND is_system = true;

CREATE OR REPLACE FUNCTION sync_system_flags_and_tags() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    -- Direction A: tags TEXT[] changed -> derive boolean flags.
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_indoor := 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_electric := 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_commute := 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));
    END IF;

    -- Direction B: is_indoor changed directly -> sync tag + junction.
    IF NEW.is_indoor IS DISTINCT FROM OLD.is_indoor THEN
        IF NEW.is_indoor AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_indoor AND 'Indoor' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Indoor');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    -- Direction C: is_electric changed directly -> sync tag + junction.
    IF NEW.is_electric IS DISTINCT FROM OLD.is_electric THEN
        IF NEW.is_electric AND NOT ('E-Bike' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'E-Bike');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_electric AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    -- Direction D: is_commute changed directly -> sync Pendeln tag + junction.
    IF NEW.is_commute IS DISTINCT FROM OLD.is_commute THEN
        IF NEW.is_commute AND NOT ('Pendeln' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Pendeln');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF NOT NEW.is_commute AND 'Pendeln' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'Pendeln');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags ON activities;
DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags_insert_junction ON activities;
DROP FUNCTION IF EXISTS sync_system_flags_and_tags_insert_junction();
CREATE TRIGGER trg_sync_system_flags_and_tags
    BEFORE UPDATE OF tags, is_indoor, is_electric, is_commute ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_flags_and_tags();

INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'migration'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'E-Bike' AND t.is_system = true
WHERE a.is_electric = true
ON CONFLICT (activity_id, tag_id) DO NOTHING;

UPDATE activities
SET tags = array_append(COALESCE(tags, '{}'), 'E-Bike')
WHERE is_electric = true
  AND NOT ('E-Bike' = ANY(COALESCE(tags, '{}')));

UPDATE tags
SET activity_count = (
    SELECT COUNT(*) FROM activity_tags at2 WHERE at2.tag_id = tags.id
)
WHERE name = 'E-Bike' AND is_system = true;
