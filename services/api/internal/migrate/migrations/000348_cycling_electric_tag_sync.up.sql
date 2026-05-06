-- 000348_cycling_electric_tag_sync.up.sql

-- E-Bike is a cycling-specific system tag. is_electric remains a generic
-- activity modifier and may be true for non-cycling activities.

SELECT pg_advisory_xact_lock(hashtext('cycling_electric_tag_sync'), 1);

INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
SELECT u.id, 'E-Bike', '#16A34A', '#FFFFFF', 'system', true, false,
       'Automatisch bei elektrischen Radfahrten'
FROM users u
ON CONFLICT (user_id, name) DO UPDATE SET
    color = CASE WHEN tags.is_system THEN EXCLUDED.color ELSE tags.color END,
    text_color = CASE WHEN tags.is_system THEN EXCLUDED.text_color ELSE tags.text_color END,
    category = 'system',
    is_system = true,
    is_auto = false,
    description = CASE WHEN tags.is_system THEN EXCLUDED.description ELSE tags.description END;

UPDATE tags
SET description = 'Automatisch bei elektrischen Radfahrten'
WHERE name = 'E-Bike' AND is_system = true AND COALESCE(description, '') = '';

CREATE OR REPLACE FUNCTION prevent_system_tag_delete() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.is_system = true AND EXISTS (SELECT 1 FROM users WHERE id = OLD.user_id) THEN
        RAISE EXCEPTION 'System-Tags können nicht gelöscht werden: %', OLD.name;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION sync_system_flags_and_tags() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- Direction A: supplied tags TEXT[] on INSERT derive boolean flags.
        IF NEW.tags IS NOT NULL THEN
            NEW.is_indoor := COALESCE(NEW.is_indoor, false) OR 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
            NEW.is_commute := COALESCE(NEW.is_commute, false) OR 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));

            IF NEW.sport_type = 'cycling' THEN
                NEW.is_electric := COALESCE(NEW.is_electric, false) OR 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
            ELSIF 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
                NEW.tags := array_remove(NEW.tags, 'E-Bike');
            END IF;
        END IF;

        -- Direction B-D: directly set flags on INSERT sync denormalized tags[].
        IF COALESCE(NEW.is_indoor, false) AND NOT ('Indoor' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Indoor');
        END IF;
        IF COALESCE(NEW.is_commute, false) AND NOT ('Pendeln' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'Pendeln');
        END IF;
        IF NEW.sport_type = 'cycling' AND COALESCE(NEW.is_electric, false) AND NOT ('E-Bike' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'E-Bike');
        ELSIF NEW.sport_type <> 'cycling' AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
        END IF;

        RETURN NEW;
    END IF;

    -- Direction A: tags TEXT[] changed -> derive boolean flags.
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_indoor := 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_commute := 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));

        IF NEW.sport_type = 'cycling' THEN
            NEW.is_electric := 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
        ELSIF 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            SELECT id INTO v_tag_id
            FROM tags
            WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true
            LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags
                SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id)
                WHERE id = v_tag_id;
            END IF;
        END IF;
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

    -- Direction C: is_electric or sport_type changed directly -> cycling-only E-Bike tag sync.
    IF NEW.is_electric IS DISTINCT FROM OLD.is_electric OR NEW.sport_type IS DISTINCT FROM OLD.sport_type THEN
        SELECT id INTO v_tag_id
        FROM tags
        WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true
        LIMIT 1;

        IF v_tag_id IS NOT NULL AND NEW.sport_type = 'cycling' AND NEW.is_electric AND NOT ('E-Bike' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'E-Bike');
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        ELSIF (NEW.sport_type <> 'cycling' OR NOT NEW.is_electric) AND 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        ELSIF v_tag_id IS NOT NULL AND NEW.sport_type <> 'cycling' AND OLD.sport_type = 'cycling' THEN
            DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
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

CREATE OR REPLACE FUNCTION sync_system_flags_and_tags_insert_junction() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF COALESCE(NEW.is_indoor, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Indoor' AND is_system = true LIMIT 1;
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    IF NEW.sport_type = 'cycling' AND COALESCE(NEW.is_electric, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    IF COALESCE(NEW.is_commute, false) THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'Pendeln' AND is_system = true LIMIT 1;
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'trigger') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
        END IF;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags ON activities;
CREATE TRIGGER trg_sync_system_flags_and_tags
    BEFORE INSERT OR UPDATE OF tags, sport_type, is_indoor, is_electric, is_commute ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_flags_and_tags();

DROP TRIGGER IF EXISTS trg_sync_system_flags_and_tags_insert_junction ON activities;
CREATE TRIGGER trg_sync_system_flags_and_tags_insert_junction
    AFTER INSERT ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_system_flags_and_tags_insert_junction();

DELETE FROM activity_tags at
USING activities a, tags t
WHERE at.activity_id = a.id
  AND at.tag_id = t.id
  AND t.name = 'E-Bike'
  AND t.is_system = true
  AND a.sport_type <> 'cycling';

UPDATE activities
SET tags = array_remove(tags, 'E-Bike')
WHERE sport_type <> 'cycling'
  AND 'E-Bike' = ANY(COALESCE(tags, '{}'));

INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'migration'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'E-Bike' AND t.is_system = true
WHERE a.sport_type = 'cycling' AND a.is_electric = true
ON CONFLICT (activity_id, tag_id) DO NOTHING;

UPDATE activities
SET tags = array_append(COALESCE(tags, '{}'), 'E-Bike')
WHERE sport_type = 'cycling'
  AND is_electric = true
  AND NOT ('E-Bike' = ANY(COALESCE(tags, '{}')))
  AND EXISTS (
      SELECT 1 FROM tags t
      WHERE t.user_id = activities.user_id
        AND t.name = 'E-Bike'
        AND t.is_system = true
  );

DELETE FROM activity_tags at
USING activities a, tags t
WHERE at.activity_id = a.id
  AND at.tag_id = t.id
  AND t.name = 'E-Bike'
  AND t.is_system = true
  AND a.sport_type = 'cycling'
  AND a.is_electric = false;

UPDATE activities
SET tags = array_remove(tags, 'E-Bike')
WHERE sport_type = 'cycling'
  AND is_electric = false
  AND 'E-Bike' = ANY(COALESCE(tags, '{}'));

UPDATE tags
SET activity_count = (
    SELECT COUNT(*) FROM activity_tags at2 WHERE at2.tag_id = tags.id
)
WHERE name = 'E-Bike' AND is_system = true;
