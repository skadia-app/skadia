DROP TRIGGER IF EXISTS trg_prevent_virtual_tag_removal ON activity_tags;
DROP FUNCTION IF EXISTS prevent_virtual_tag_removal();
DROP TRIGGER IF EXISTS trg_sync_virtual_tag_insert_junction ON activities;
DROP FUNCTION IF EXISTS sync_virtual_tag_insert_junction();
DROP TRIGGER IF EXISTS trg_sync_virtual_tag_insert ON activities;
DROP FUNCTION IF EXISTS sync_virtual_tag_insert();
DROP TRIGGER IF EXISTS trg_sync_virtual_tag ON activities;
DROP FUNCTION IF EXISTS sync_virtual_tag();

-- Tags entfernen
UPDATE activities SET tags = array_remove(tags, 'virtuell')
WHERE 'virtuell' = ANY(tags);

DELETE FROM activity_tags WHERE tag_id IN (
    SELECT id FROM tags WHERE name = 'virtuell'
);
