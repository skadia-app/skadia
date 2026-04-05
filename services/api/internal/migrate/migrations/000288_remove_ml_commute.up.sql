-- 000288_remove_ml_commute.up.sql
-- ML-Pendelerkennung entfernen, commute_auto system_type einführen

DROP TABLE IF EXISTS commute_clusters;

ALTER TABLE user_enrichment_settings
  DROP COLUMN IF EXISTS enable_commute_detection;

DELETE FROM automation_rules WHERE system_type = 'commute_sync';

ALTER TABLE automation_rules DROP CONSTRAINT IF EXISTS automation_rules_system_type_check;
ALTER TABLE automation_rules ADD CONSTRAINT automation_rules_system_type_check
  CHECK (system_type IN (
    'electric_sync', 'round_trip', 'frost', 'nacht',
    'virtual_sync', 'indoor_sync', 'commute_auto'
  ));
