-- 000186_devices_backfill.down.sql
UPDATE activity_sensors SET device_id = NULL;
UPDATE activities SET recording_device_id = NULL;
DELETE FROM devices;
