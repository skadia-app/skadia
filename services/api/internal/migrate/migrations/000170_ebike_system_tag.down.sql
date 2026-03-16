-- Trigger entfernen
DROP TRIGGER IF EXISTS trg_sync_ebike_from_tags ON activities;
DROP FUNCTION IF EXISTS sync_ebike_from_tags();
DROP TRIGGER IF EXISTS trg_prevent_system_tag_delete ON tags;
DROP FUNCTION IF EXISTS prevent_system_tag_delete();

-- E-Bike-Tags aus activity_tags entfernen
DELETE FROM activity_tags WHERE tag_id IN (SELECT id FROM tags WHERE name = 'E-Bike' AND is_system = true);
-- E-Bike System-Tags löschen
DELETE FROM tags WHERE name = 'E-Bike' AND is_system = true;
-- TEXT[] cache bereinigen
UPDATE activities SET tags = array_remove(tags, 'E-Bike') WHERE 'E-Bike' = ANY(tags);
-- is_ebike NICHT zurücksetzen — die Spalte enthält die ursprünglichen Import-Werte
-- und sollte nach Rollback unverändert bleiben (Parser setzt sie korrekt).
