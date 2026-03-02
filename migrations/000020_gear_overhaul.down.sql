-- Rollback: Gear Overhaul

-- Presets
DROP TABLE IF EXISTS component_type_presets;

-- Service-Events
DROP TABLE IF EXISTS gear_service_events;

-- Kettenrotation
DROP TABLE IF EXISTS chain_rotation_members;
DROP TABLE IF EXISTS chain_rotation_groups;

-- Komponenten-Transfers
DROP TABLE IF EXISTS component_assignments;

-- gear_components: Neue Spalten entfernen
ALTER TABLE gear_components DROP CONSTRAINT IF EXISTS gear_components_status_check;
ALTER TABLE gear_components DROP COLUMN IF EXISTS status;
ALTER TABLE gear_components DROP COLUMN IF EXISTS serial_number;
ALTER TABLE gear_components DROP COLUMN IF EXISTS weight_g;
ALTER TABLE gear_components DROP COLUMN IF EXISTS purchase_date;
ALTER TABLE gear_components DROP COLUMN IF EXISTS purchase_price;
ALTER TABLE gear_components DROP COLUMN IF EXISTS total_time_s;
ALTER TABLE gear_components DROP COLUMN IF EXISTS total_elevation_gain_m;

-- gear: Neue Spalten entfernen
ALTER TABLE gear DROP CONSTRAINT IF EXISTS gear_status_check;
ALTER TABLE gear DROP COLUMN IF EXISTS status;
ALTER TABLE gear DROP COLUMN IF EXISTS total_elevation_gain_m;
