-- 000295_fix_system_tag_category.up.sql
--
-- System-Tags (is_system=true) sollen category='system' haben, nicht das Default 'custom'.
-- Betroffen: Tags die nach Migration 000179 ohne explizites category-Feld insertet wurden
-- (Indoor via 000261, Pendeln via 000268, virtuell via seed in 000220, manuell via stage1-Enrichment).
UPDATE tags
SET category = 'system'
WHERE is_system = true
  AND category = 'custom';
