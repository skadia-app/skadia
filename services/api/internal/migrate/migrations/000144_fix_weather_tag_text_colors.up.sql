-- Fix text_color for weather tags that were seeded with hardcoded #FFFFFF
-- regardless of background color, making light-colored tags unreadable.
-- Luminance formula: (0.299*R + 0.587*G + 0.114*B) / 255 > 0.5 → dark text
UPDATE tags
SET text_color = CASE
    WHEN (
        0.299 * ('x' || lpad(substr(color, 2, 2), 2, '0'))::bit(8)::int +
        0.587 * ('x' || lpad(substr(color, 4, 2), 2, '0'))::bit(8)::int +
        0.114 * ('x' || lpad(substr(color, 6, 2), 2, '0'))::bit(8)::int
    ) / 255.0 > 0.5
    THEN '#1a1a1a'
    ELSE '#ffffff'
END
WHERE category = 'weather'
  AND length(color) = 7
  AND color LIKE '#%';
