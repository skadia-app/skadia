-- Add ANT+ device number to activity_sensors.
-- serial_number already exists since migration 000004.
ALTER TABLE activity_sensors ADD COLUMN IF NOT EXISTS ant_device_number INTEGER;

CREATE INDEX IF NOT EXISTS activity_sensors_ant_device_number_idx
    ON activity_sensors (ant_device_number) WHERE ant_device_number IS NOT NULL;
