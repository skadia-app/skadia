-- 000262_fix_strength_indoor_backfill.down.sql
-- Reverting this is destructive (removes indoor classification for strength activities).
-- Only run if you need to roll back to pre-000262 state.

UPDATE activities SET tags = array_remove(tags, 'Indoor')
WHERE sport_type = 'strength' AND is_indoor = true
  AND 'Indoor' = ANY(COALESCE(tags, '{}'));

DELETE FROM activity_tags
WHERE activity_id IN (SELECT id FROM activities WHERE sport_type = 'strength')
  AND tag_id IN (SELECT id FROM tags WHERE name = 'Indoor' AND is_system = true);

UPDATE activities SET is_indoor = false
WHERE sport_type = 'strength';

UPDATE tags SET activity_count = (
    SELECT COUNT(*) FROM activity_tags at2 WHERE at2.tag_id = tags.id
)
WHERE name = 'Indoor' AND is_system = true;
