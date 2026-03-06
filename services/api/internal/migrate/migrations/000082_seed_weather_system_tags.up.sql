-- Seed weather system tags for all users
-- These are automatically assigned during Stage 1 enrichment
INSERT INTO tags (user_id, name, color, text_color, category, is_system, is_auto, description)
SELECT DISTINCT u.id,
       tag_names.name,
       tag_colors.color,
       '#FFFFFF',
       'weather',
       true,
       false,
       'Automatisch erkannt aus Wetterdaten'
FROM users u
CROSS JOIN (VALUES
    ('Sonnig', '#FCD34D'),
    ('Bewölkt', '#9CA3AF'),
    ('Regen', '#3B82F6'),
    ('Schnee', '#F3F4F6'),
    ('Nebel', '#D1D5DB'),
    ('Gewitter', '#7C3AED')
) AS tag_names(name, color)
WHERE u.is_active = true
ON CONFLICT (user_id, name) DO NOTHING;

-- Backfill: Link existing activities with weather_tags to their respective tags
INSERT INTO activity_tags (activity_id, tag_id, assigned_by)
SELECT a.id, t.id, 'auto'
FROM activities a
JOIN tags t ON t.user_id = a.user_id AND t.category = 'weather' AND t.is_system = true
WHERE a.weather_tags IS NOT NULL
  AND a.weather_tags != '{}'
  AND t.name ILIKE ANY (SELECT unnest(a.weather_tags))
ON CONFLICT (activity_id, tag_id) DO NOTHING;
