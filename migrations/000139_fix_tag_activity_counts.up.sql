-- Fix tag activity_count denormalization
-- This migration recalculates all activity_count values from the activity_tags junction table

UPDATE tags t
SET activity_count = (
  SELECT COUNT(DISTINCT activity_id)
  FROM activity_tags
  WHERE tag_id = t.id
);

-- Verify all counts are now populated correctly
-- SELECT name, activity_count FROM tags WHERE activity_count > 0 ORDER BY activity_count DESC;
