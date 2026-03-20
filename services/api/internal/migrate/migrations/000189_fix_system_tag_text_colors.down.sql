-- Revert Rundtour text_color to original (incorrect) value
UPDATE tags
SET text_color = '#FFFFFF'
WHERE name = 'Rundtour'
  AND is_system = true
  AND color = '#10B981'
  AND text_color = '#1a1a1a';

