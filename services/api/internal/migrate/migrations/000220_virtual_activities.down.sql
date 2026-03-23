ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
  CHECK (system_type IN ('ebike_sync', 'round_trip', 'frost', 'nacht'));

ALTER TABLE activities DROP COLUMN IF EXISTS exclude_from_stats;
ALTER TABLE activities DROP COLUMN IF EXISTS exclude_from_heatmap;
ALTER TABLE activities DROP COLUMN IF EXISTS is_virtual;
