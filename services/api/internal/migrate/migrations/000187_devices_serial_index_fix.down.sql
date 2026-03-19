-- 000187_devices_serial_index_fix.down.sql
DROP INDEX IF EXISTS idx_devices_serial;
CREATE UNIQUE INDEX idx_devices_serial
    ON devices (user_id, type, manufacturer_id, serial_number)
    WHERE serial_number IS NOT NULL;
