-- 000185_devices.down.sql
ALTER TABLE activity_sensors DROP COLUMN IF EXISTS device_id;
ALTER TABLE activities DROP COLUMN IF EXISTS recording_device_id;
DROP TABLE IF EXISTS devices;
