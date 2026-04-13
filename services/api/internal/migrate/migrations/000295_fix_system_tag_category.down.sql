-- 000295_fix_system_tag_category.down.sql
-- Revert: system-Tags zurück auf 'custom' (Ausgangszustand vor dem Fix)
UPDATE tags
SET category = 'custom'
WHERE is_system = true
  AND category = 'system';
