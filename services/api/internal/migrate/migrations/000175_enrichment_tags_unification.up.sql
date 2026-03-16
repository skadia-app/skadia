-- 000173_enrichment_tags_unification.up.sql

-- 1. activity_tag_removals table
CREATE TABLE activity_tag_removals (
    activity_id UUID NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    tag_id      UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    removed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (activity_id, tag_id)
);

-- 2. Convert "Rundtour" from is_auto=true to is_auto=false (trigger-managed like E-Bike)
UPDATE tags
SET is_auto = false, conditions = NULL, condition_groups = NULL, conditions_logic = NULL
WHERE name = 'Rundtour' AND is_system = true;

-- 3. Fix Schnee text_color (white-on-white → dark gray on light gray)
UPDATE tags
SET text_color = '#1F2937'
WHERE name = 'Schnee' AND is_system = true AND category = 'weather';

-- 4. Add weather_auto_tags preference to users table
ALTER TABLE users
    ADD COLUMN weather_auto_tags BOOLEAN NOT NULL DEFAULT true;

COMMENT ON COLUMN users.weather_auto_tags IS
    'When true, weather enrichment automatically assigns weather system tags (Sonnig, Regen, etc.) to activities';

-- 5. Trigger: sync is_round_trip → activity_tags (Rundtour)
CREATE OR REPLACE FUNCTION sync_rundtour_to_tags() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
    v_removed BOOLEAN;
BEGIN
    -- Prevent infinite recursion with sync_rundtour_from_tags
    IF pg_trigger_depth() > 1 THEN
        RETURN NEW;
    END IF;

    -- Only act when is_round_trip actually changed
    IF OLD.is_round_trip IS NOT DISTINCT FROM NEW.is_round_trip THEN
        RETURN NEW;
    END IF;

    SELECT id INTO v_tag_id
    FROM tags
    WHERE name = 'Rundtour' AND user_id = NEW.user_id AND is_system = true;

    IF v_tag_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF NEW.is_round_trip = true THEN
        -- Check if user has explicitly removed this tag
        SELECT EXISTS(
            SELECT 1 FROM activity_tag_removals
            WHERE activity_id = NEW.id AND tag_id = v_tag_id
        ) INTO v_removed;

        IF NOT v_removed THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
            VALUES (NEW.id, v_tag_id, 'enrichment')
            ON CONFLICT (activity_id, tag_id) DO NOTHING;

            -- Sync TEXT[] cache
            NEW.tags := array_append(
                array_remove(NEW.tags, 'Rundtour'),
                'Rundtour'
            );

            -- Update activity_count
            UPDATE tags
            SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id)
            WHERE id = v_tag_id;
        END IF;
    ELSE
        -- is_round_trip set to false → remove tag
        DELETE FROM activity_tags
        WHERE activity_id = NEW.id AND tag_id = v_tag_id;

        NEW.tags := array_remove(NEW.tags, 'Rundtour');

        UPDATE tags
        SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = v_tag_id)
        WHERE id = v_tag_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_rundtour_to_tags
    BEFORE UPDATE OF is_round_trip ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_rundtour_to_tags();

-- 6. Trigger: activity_tags insert/delete → sync is_round_trip on activities
CREATE OR REPLACE FUNCTION sync_rundtour_from_tags() RETURNS TRIGGER AS $$
DECLARE
    v_tag_name TEXT;
BEGIN
    -- Prevent infinite recursion with sync_rundtour_to_tags
    IF pg_trigger_depth() > 1 THEN
        IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'INSERT' THEN
        SELECT name INTO v_tag_name FROM tags WHERE id = NEW.tag_id;
        IF v_tag_name = 'Rundtour' THEN
            UPDATE activities SET is_round_trip = true WHERE id = NEW.activity_id;
        END IF;
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        SELECT name INTO v_tag_name FROM tags WHERE id = OLD.tag_id;
        IF v_tag_name = 'Rundtour' THEN
            UPDATE activities SET is_round_trip = false WHERE id = OLD.activity_id;
        END IF;
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_rundtour_from_tags
    AFTER INSERT OR DELETE ON activity_tags
    FOR EACH ROW
    EXECUTE FUNCTION sync_rundtour_from_tags();

-- 7. Backfill: existing is_round_trip=true → insert Rundtour tag
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'enrichment'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'Rundtour' AND t.is_system = true
WHERE a.is_round_trip = true
ON CONFLICT (activity_id, tag_id) DO NOTHING;

-- Sync TEXT[] cache for Rundtour
UPDATE activities
SET tags = array_append(tags, 'Rundtour')
WHERE is_round_trip = true
  AND NOT ('Rundtour' = ANY(tags));

-- 8. Backfill: existing activity_weather.tags → insert weather system tags
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT DISTINCT aw_act.activity_id, t.id, 'enrichment'
FROM (
    SELECT aw.activity_id, a.user_id, unnest(aw.tags) AS weather_tag
    FROM activity_weather aw
    JOIN activities a ON a.id = aw.activity_id
    WHERE aw.tags IS NOT NULL AND array_length(aw.tags, 1) > 0
) aw_act
JOIN tags t ON t.user_id = aw_act.user_id
    AND t.is_system = true
    AND t.category = 'weather'
    AND lower(t.name) = aw_act.weather_tag
ON CONFLICT (activity_id, tag_id) DO NOTHING;

-- Sync TEXT[] cache for weather tags
UPDATE activities a
SET tags = (
    SELECT array_agg(DISTINCT tag_name ORDER BY tag_name)
    FROM (
        SELECT unnest(a.tags) AS tag_name
        UNION
        SELECT t.name
        FROM activity_tags at2
        JOIN tags t ON t.id = at2.tag_id
        WHERE at2.activity_id = a.id AND t.category = 'weather' AND t.is_system = true
    ) combined
)
WHERE a.id IN (
    SELECT DISTINCT at2.activity_id
    FROM activity_tags at2
    JOIN tags t ON t.id = at2.tag_id
    WHERE t.category = 'weather' AND t.is_system = true
);

-- 9. Update activity_count for all weather + Rundtour system tags
UPDATE tags t
SET activity_count = (SELECT COUNT(*) FROM activity_tags WHERE tag_id = t.id)
WHERE t.is_system = true AND (t.category = 'weather' OR t.name = 'Rundtour');
