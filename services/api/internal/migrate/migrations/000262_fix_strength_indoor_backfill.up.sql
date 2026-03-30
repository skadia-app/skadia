-- 000262_fix_strength_indoor_backfill.up.sql
-- Corrects missing 'strength' sport_type in 000261 indoor backfill.
-- The Go taxonomy has IsIndoor=true for 'strength', but 000261's WHERE clause omitted it.

-- Step 1: Mark strength activities as indoor
UPDATE activities SET is_indoor = true
WHERE sport_type = 'strength' AND is_indoor = false;

-- Step 2: Backfill Indoor tag for strength activities
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'migration'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.name = 'Indoor' AND t.is_system = true
WHERE a.sport_type = 'strength' AND a.is_indoor = true
ON CONFLICT DO NOTHING;

-- Step 3: Add 'Indoor' to tags TEXT[] array
UPDATE activities SET tags = array_append(tags, 'Indoor')
WHERE sport_type = 'strength' AND is_indoor = true
  AND NOT ('Indoor' = ANY(COALESCE(tags, '{}')));

-- Step 4: Recompute activity_count on affected Indoor tags
UPDATE tags SET activity_count = (
    SELECT COUNT(*) FROM activity_tags at2 WHERE at2.tag_id = tags.id
)
WHERE name = 'Indoor' AND is_system = true;
