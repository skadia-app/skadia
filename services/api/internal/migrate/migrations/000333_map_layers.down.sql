-- 000333_map_layers.down.sql
-- WARNING: This migration drops user data in user_map_layers.
-- Take a snapshot before rolling back:
--   pg_dump -t user_map_layers -t map_layers -t system_layer_visibility > pre_rollback_$(date +%s).sql

DO $$
BEGIN
    RAISE NOTICE 'Run pg_dump for user_map_layers, map_layers, system_layer_visibility before rolling back — user data will be lost otherwise';
END $$;

-- Undo the audit-log entry inserted by up.sql step 7.
DELETE FROM auth_audit_log WHERE details->>'migration' = '000333';

DROP TRIGGER IF EXISTS trg_map_layers_updated_at              ON map_layers;
DROP TRIGGER IF EXISTS trg_system_layer_visibility_updated_at ON system_layer_visibility;
DROP TRIGGER IF EXISTS trg_user_map_layers_updated_at         ON user_map_layers;

DROP TABLE IF EXISTS user_map_layers;
DROP TABLE IF EXISTS system_layer_visibility;
DROP TABLE IF EXISTS map_layers;
