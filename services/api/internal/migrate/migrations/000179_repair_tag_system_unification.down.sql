-- Down migration: revert tag system unification repair
DROP TRIGGER IF EXISTS trg_sync_rundtour_to_tags ON activities;
DROP TRIGGER IF EXISTS trg_sync_rundtour_from_tags ON activity_tags;

DROP INDEX IF EXISTS uq_automation_rules_system_type;

DELETE FROM automation_rules WHERE is_system = true;

ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules DROP COLUMN IF EXISTS system_type;
ALTER TABLE automation_rules DROP COLUMN IF EXISTS is_system;
