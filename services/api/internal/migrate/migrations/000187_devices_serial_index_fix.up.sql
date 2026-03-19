-- 000187_devices_serial_index_fix.up.sql
-- Serial number allein reicht zur Identifikation — manufacturer_id raus aus dem Index.
-- Dasselbe physische Gerät wird manchmal als garmin(1) und manchmal als ManufacturerInvalid(-1)
-- aufgezeichnet. Mit manufacturer_id im Index entstehen Duplikate.

DROP INDEX IF EXISTS idx_devices_serial;
CREATE UNIQUE INDEX idx_devices_serial
    ON devices (user_id, type, serial_number)
    WHERE serial_number IS NOT NULL;
