-- 1. E-Bike System-Tag für alle existierenden User anlegen
INSERT INTO tags (user_id, name, color, text_color, is_system, is_auto, description)
SELECT DISTINCT u.id, 'E-Bike', '#16A34A', '#FFFFFF', true, false,
       'Automatisch bei E-Bike-Aktivitäten (Sync mit is_ebike)'
FROM users u
WHERE NOT EXISTS (
    SELECT 1 FROM tags t WHERE t.user_id = u.id AND t.name = 'E-Bike'
);

-- 2. Backfill: Alle Aktivitäten mit is_ebike=true bekommen den E-Bike-Tag
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'migration'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'E-Bike'
WHERE a.is_ebike = true
ON CONFLICT (activity_id, tag_id) DO NOTHING;

-- 3. activity_count aktualisieren
UPDATE tags SET activity_count = (
    SELECT COUNT(DISTINCT activity_id) FROM activity_tags WHERE tag_id = tags.id
)
WHERE name = 'E-Bike';

-- 4. Denormalized TEXT[] cache aktualisieren
UPDATE activities SET tags = array_append(COALESCE(tags, '{}'), 'E-Bike')
WHERE is_ebike = true AND NOT ('E-Bike' = ANY(COALESCE(tags, '{}')));

-- 5. PostgreSQL Trigger: Bidirektionale Sync tags TEXT[] ↔ is_ebike
-- Feuert bei jedem UPDATE auf activities.tags und setzt is_ebike entsprechend.
-- Deckt ALLE Write-Paths ab: UpdateActivity, UpdateEnrichmentFields,
-- AssignActivitiesToTag, UnassignActivitiesFromTag, reprocessTags.
CREATE OR REPLACE FUNCTION sync_ebike_from_tags() RETURNS TRIGGER AS $$
BEGIN
    -- Nur reagieren wenn sich tags tatsächlich geändert hat
    IF NEW.tags IS DISTINCT FROM OLD.tags THEN
        NEW.is_ebike := ('E-Bike' = ANY(COALESCE(NEW.tags, '{}')));
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_sync_ebike_from_tags
    BEFORE UPDATE OF tags ON activities
    FOR EACH ROW
    EXECUTE FUNCTION sync_ebike_from_tags();

-- 6. Löschschutz: is_system Tags dürfen nicht gelöscht werden
CREATE OR REPLACE FUNCTION prevent_system_tag_delete() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.is_system = true THEN
        RAISE EXCEPTION 'System-Tags können nicht gelöscht werden: %', OLD.name;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_prevent_system_tag_delete
    BEFORE DELETE ON tags
    FOR EACH ROW
    EXECUTE FUNCTION prevent_system_tag_delete();
