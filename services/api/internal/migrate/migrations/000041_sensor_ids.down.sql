DROP INDEX IF EXISTS activity_sensors_ant_device_number_idx;
ALTER TABLE activity_sensors DROP COLUMN IF EXISTS ant_device_number;
