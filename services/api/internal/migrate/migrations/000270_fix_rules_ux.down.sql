-- Revert naming
UPDATE automation_rules SET name = 'Indoor-Sync' WHERE system_type = 'indoor_sync';
UPDATE automation_rules SET name = 'Pendeln-Sync' WHERE system_type = 'commute_sync';
UPDATE automation_rules SET name = 'virtuell' WHERE system_type = 'virtual_sync';
-- Note: seed rules/tags not reverted (user may have modified them)
