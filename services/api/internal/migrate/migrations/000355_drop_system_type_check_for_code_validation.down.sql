-- Phase 4b.A.1 down — restore the historical CHECK whitelist + drop version column.
--
-- WARNING: any system_type values inserted after the up-migration (e.g.
-- 'lift_detected' from Phase 4b.B) will cause this DOWN to fail unless those
-- rows are deleted first. This is intentional — Down is purely for emergency
-- rollback in test/CI, not for routine production downgrades.

DROP INDEX IF EXISTS idx_users_system_artifacts_version;
ALTER TABLE users DROP COLUMN IF EXISTS system_artifacts_version;

ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
  CHECK (system_type IN (
    'electric_sync', 'round_trip', 'frost', 'nacht',
    'virtual_sync', 'indoor_sync', 'commute_auto'
  ));
