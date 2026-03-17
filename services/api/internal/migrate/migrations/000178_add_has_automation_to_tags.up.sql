-- 000178_add_has_automation_to_tags.up.sql
-- has_automation wurde in 000176 definiert aber nicht angewendet (DB war bereits auf 177).
ALTER TABLE tags ADD COLUMN IF NOT EXISTS has_automation BOOLEAN NOT NULL DEFAULT false;

-- Initialwert: Tags die in aktiven System-Rules referenziert werden
UPDATE tags t SET has_automation = true
WHERE EXISTS (
  SELECT 1 FROM automation_rules ar
  WHERE ar.user_id = t.user_id AND ar.is_active = true
    AND ar.actions::text LIKE '%"tag_id":"' || t.id::text || '"%'
);
