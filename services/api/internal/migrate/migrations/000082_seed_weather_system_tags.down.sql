-- Remove activity_tags links for weather system tags
DELETE FROM activity_tags WHERE tag_id IN (
    SELECT id FROM tags WHERE category = 'weather' AND is_system = true
);

-- Remove weather system tags
DELETE FROM tags WHERE category = 'weather' AND is_system = true;
