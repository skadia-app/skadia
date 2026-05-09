-- 000353_system_modifier_tag_sync_hardening.down.sql

SELECT pg_advisory_xact_lock(hashtext('system_modifier_tag_sync_hardening'), 1);

DROP FUNCTION IF EXISTS skadia_resync_system_modifier_tags(UUID[]);
DROP INDEX IF EXISTS idx_activity_tags_tag_id_activity_id;

-- Triggers from 000350_cycling_electric_tag_sync.up.sql and
-- 000221_virtual_system_tag.up.sql are restored by re-running their bodies.
-- The down migration intentionally does not delete activity, tag, or junction
-- data, only the new function/index introduced by this migration.

-- Restore previous sync_system_flags_and_tags() body (matches 000350 main).
CREATE OR REPLACE FUNCTION sync_system_flags_and_tags() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.tags IS NOT NULL THEN
            NEW.is_indoor := COALESCE(NEW.is_indoor, false) OR 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
            NEW.is_commute := COALESCE(NEW.is_commute, false) OR 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));
            IF NEW.sport_type = 'cycling' THEN
                NEW.is_electric := COALESCE(NEW.is_electric, false) OR 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
            ELSIF 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
                NEW.tags := array_remove(NEW.tags, 'E-Bike');
            END IF;
        END IF;

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

    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_indoor := 'Indoor' = ANY(COALESCE(NEW.tags, '{}'));
        NEW.is_commute := 'Pendeln' = ANY(COALESCE(NEW.tags, '{}'));

        IF NEW.sport_type = 'cycling' THEN
            NEW.is_electric := 'E-Bike' = ANY(COALESCE(NEW.tags, '{}'));
        ELSIF 'E-Bike' = ANY(COALESCE(NEW.tags, '{}')) THEN
            NEW.tags := array_remove(NEW.tags, 'E-Bike');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
                UPDATE tags SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id) WHERE id = v_tag_id;
            END IF;
        END IF;
    END IF;

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

    IF NEW.is_electric IS DISTINCT FROM OLD.is_electric OR NEW.sport_type IS DISTINCT FROM OLD.sport_type THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'E-Bike' AND is_system = true LIMIT 1;

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
$$;

CREATE OR REPLACE FUNCTION sync_system_flags_and_tags_insert_junction() RETURNS TRIGGER
LANGUAGE plpgsql
SET search_path = public
AS $$
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
$$;

-- Restore previous sync_virtual_tag()/insert/insert_junction bodies (000221).
CREATE OR REPLACE FUNCTION sync_virtual_tag() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF NEW.is_virtual IS DISTINCT FROM OLD.is_virtual THEN
        IF NEW.is_virtual THEN
            IF NOT ('virtuell' = ANY(COALESCE(NEW.tags, '{}'))) THEN
                NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'virtuell');
            END IF;
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'system') ON CONFLICT (activity_id, tag_id) DO NOTHING;
            END IF;
        ELSE
            NEW.tags := array_remove(NEW.tags, 'virtuell');
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION sync_virtual_tag_insert() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF NEW.is_virtual THEN
        IF NOT ('virtuell' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'virtuell');
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION sync_virtual_tag_insert_junction() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF NEW.is_virtual THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by) VALUES (NEW.id, v_tag_id, 'system') ON CONFLICT (activity_id, tag_id) DO NOTHING;
        END IF;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
