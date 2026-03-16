-- 000173_enrichment_tags_unification.down.sql

DROP TRIGGER IF EXISTS trg_sync_rundtour_from_tags ON activity_tags;
DROP FUNCTION IF EXISTS sync_rundtour_from_tags();

DROP TRIGGER IF EXISTS trg_sync_rundtour_to_tags ON activities;
DROP FUNCTION IF EXISTS sync_rundtour_to_tags();

-- Revert Rundtour to autotag
UPDATE tags
SET is_auto = true,
    conditions = '{"operator":"AND","rules":[{"field":"is_round_trip","op":"eq","value":true}]}'::jsonb
WHERE name = 'Rundtour' AND is_system = true;

-- Revert Schnee text_color
UPDATE tags
SET text_color = '#FFFFFF'
WHERE name = 'Schnee' AND is_system = true AND category = 'weather';

-- Remove weather tags from activity_tags (revert backfill)
DELETE FROM activity_tags
WHERE tag_id IN (SELECT id FROM tags WHERE is_system = true AND category = 'weather')
  AND assigned_by = 'enrichment';

-- Remove Rundtour entries from activity_tags that were backfilled
DELETE FROM activity_tags
WHERE tag_id IN (SELECT id FROM tags WHERE name = 'Rundtour' AND is_system = true)
  AND assigned_by = 'enrichment';

-- Sync TEXT[] cache (rebuild from remaining activity_tags)
UPDATE activities a
SET tags = COALESCE((
    SELECT array_agg(t.name ORDER BY t.name)
    FROM activity_tags at2
    JOIN tags t ON t.id = at2.tag_id
    WHERE at2.activity_id = a.id
), '{}');

ALTER TABLE users DROP COLUMN IF EXISTS weather_auto_tags;

DROP TABLE IF EXISTS activity_tag_removals;
