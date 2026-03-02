-- Rollback Phase 6: Gear + Automation Rules
DROP TABLE IF EXISTS automation_rules;
ALTER TABLE activities DROP COLUMN IF EXISTS gear_id;
DROP TABLE IF EXISTS gear_service_intervals;
DROP TABLE IF EXISTS gear_components;
DROP TABLE IF EXISTS gear;
