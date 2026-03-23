-- 1. Backfill: Alle Aktivitäten mit is_virtual=true bekommen den "virtuell"-Tag
-- (Tag wurde bereits in Migration 000220 via EnsureDefaultSystemRules angelegt)
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'migration'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'virtuell'
WHERE a.is_virtual = true
ON CONFLICT (activity_id, tag_id) DO NOTHING;

-- 2. activity_count aktualisieren
UPDATE tags SET activity_count = (
    SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = tags.id
)
WHERE name = 'virtuell';

-- 3. Denormalized TEXT[] cache aktualisieren
UPDATE activities SET tags = array_append(COALESCE(tags, '{}'), 'virtuell')
WHERE is_virtual = true AND NOT ('virtuell' = ANY(COALESCE(tags, '{}')));

-- 4. PostgreSQL Trigger: Sync is_virtual → tags TEXT[] + activity_tags
-- Feuert bei UPDATE auf is_virtual. Tag folgt der Spalte (hardcoded, nicht entfernbar).
CREATE OR REPLACE FUNCTION sync_virtual_tag() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF NEW.is_virtual IS DISTINCT FROM OLD.is_virtual THEN
        IF NEW.is_virtual THEN
            -- Tag in tags[] einfügen
            IF NOT ('virtuell' = ANY(COALESCE(NEW.tags, '{}'))) THEN
                NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'virtuell');
            END IF;
            -- Junction table: activity_tags verknüpfen
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
            IF v_tag_id IS NOT NULL THEN
                INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
                VALUES (NEW.id, v_tag_id, 'system')
                ON CONFLICT (activity_id, tag_id) DO NOTHING;
            END IF;
        ELSE
            -- Tag aus tags[] entfernen
            NEW.tags := array_remove(NEW.tags, 'virtuell');
            -- Junction table: activity_tags entfernen
            SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
            IF v_tag_id IS NOT NULL THEN
                DELETE FROM activity_tags WHERE activity_id = NEW.id AND tag_id = v_tag_id;
            END IF;
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_virtual_tag
    BEFORE UPDATE OF is_virtual ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_virtual_tag();

-- 5. INSERT-Trigger: tags[] + activity_tags bei neuen virtuellen Aktivitäten
CREATE OR REPLACE FUNCTION sync_virtual_tag_insert() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF NEW.is_virtual THEN
        -- Tag in tags[] einfügen
        IF NOT ('virtuell' = ANY(COALESCE(NEW.tags, '{}'))) THEN
            NEW.tags := array_append(COALESCE(NEW.tags, '{}'), 'virtuell');
        END IF;
        -- Junction table: activity_tags verknüpfen (deferred — ID muss existieren)
        -- Nutzt einen AFTER INSERT Trigger stattdessen, weil NEW.id beim BEFORE INSERT
        -- noch nicht in der DB existiert und der FK-Constraint fehlschlagen würde.
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_virtual_tag_insert
    BEFORE INSERT ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_virtual_tag_insert();

-- 6. AFTER INSERT Trigger: activity_tags Junction für neue virtuelle Aktivitäten
CREATE OR REPLACE FUNCTION sync_virtual_tag_insert_junction() RETURNS TRIGGER AS $$
DECLARE
    v_tag_id UUID;
BEGIN
    IF NEW.is_virtual THEN
        SELECT id INTO v_tag_id FROM tags WHERE user_id = NEW.user_id AND name = 'virtuell';
        IF v_tag_id IS NOT NULL THEN
            INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
            VALUES (NEW.id, v_tag_id, 'system')
            ON CONFLICT (activity_id, tag_id) DO NOTHING;
        END IF;
    END IF;
    RETURN NULL; -- AFTER trigger, return value ignored
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_virtual_tag_insert_junction
    AFTER INSERT ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_virtual_tag_insert_junction();
