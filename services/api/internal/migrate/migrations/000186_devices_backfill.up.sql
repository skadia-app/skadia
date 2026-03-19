-- 000186_devices_backfill.up.sql

-- 1. Sensor-Devices aus activity_sensors extrahieren
INSERT INTO devices (user_id, type, manufacturer, manufacturer_id, product_name,
                     product_id, serial_number, ant_device_number, device_types,
                     source_type, first_seen_at, last_seen_at)
SELECT a.user_id, 'sensor',
       s.manufacturer, s.manufacturer_id::INTEGER, s.product_name,
       s.garmin_product_id, s.serial_number, s.ant_device_number,
       s.all_device_types,
       CASE WHEN s.source_type IN ('antplus', 'bluetooth') THEN s.source_type ELSE NULL END,
       MIN(a.started_at), MAX(a.started_at)
FROM activity_sensors s
JOIN activities a ON a.id = s.activity_id
WHERE s.manufacturer IS NOT NULL OR s.serial_number IS NOT NULL OR s.ant_device_number IS NOT NULL
GROUP BY a.user_id, s.manufacturer, s.manufacturer_id, s.product_name,
         s.garmin_product_id, s.serial_number, s.ant_device_number,
         s.all_device_types, s.source_type
ON CONFLICT DO NOTHING;

-- 2. Recorder-Devices aus activities.source_device extrahieren
INSERT INTO devices (user_id, type, manufacturer, product_name, first_seen_at, last_seen_at)
SELECT a.user_id, 'recorder',
       CASE a.source
           WHEN 'Garmin' THEN 'garmin'
           WHEN 'Wahoo' THEN 'wahoo_fitness'
           WHEN 'Suunto' THEN 'suunto'
           WHEN 'Polar' THEN 'polar_electro'
           WHEN 'Sigma Sport' THEN 'sigmasport'
           WHEN 'Hammerhead' THEN 'hammerhead'
           WHEN 'Coros' THEN 'coros'
           WHEN 'Bryton' THEN 'bryton'
           WHEN 'Strava' THEN 'strava'
           WHEN 'Zwift' THEN 'zwift'
           ELSE LOWER(COALESCE(a.source, ''))
       END,
       a.source_device,
       MIN(a.started_at), MAX(a.started_at)
FROM activities a
WHERE a.source_device IS NOT NULL AND a.source_device != ''
GROUP BY a.user_id, a.source, a.source_device
ON CONFLICT DO NOTHING;

-- 3. activity_sensors.device_id setzen (per serial_number Match)
UPDATE activity_sensors s SET device_id = d.id
FROM devices d
JOIN activities a ON a.id = s.activity_id
WHERE d.type = 'sensor'
  AND d.user_id = a.user_id
  AND d.serial_number IS NOT NULL
  AND d.serial_number = s.serial_number
  AND d.manufacturer_id = s.manufacturer_id::INTEGER
  AND s.device_id IS NULL;

-- 4. activity_sensors.device_id setzen (per ant_device_number Match)
UPDATE activity_sensors s SET device_id = d.id
FROM devices d
JOIN activities a ON a.id = s.activity_id
WHERE d.type = 'sensor'
  AND d.user_id = a.user_id
  AND d.ant_device_number IS NOT NULL
  AND d.ant_device_number = s.ant_device_number
  AND d.serial_number IS NULL
  AND d.manufacturer_id = s.manufacturer_id::INTEGER
  AND s.device_id IS NULL;

-- 5. activity_sensors.device_id setzen (per product_name Match)
UPDATE activity_sensors s SET device_id = d.id
FROM devices d
JOIN activities a ON a.id = s.activity_id
WHERE d.type = 'sensor'
  AND d.user_id = a.user_id
  AND d.serial_number IS NULL AND d.ant_device_number IS NULL
  AND COALESCE(d.manufacturer_id, 0) = COALESCE(s.manufacturer_id::INTEGER, 0)
  AND d.product_name = s.product_name
  AND s.device_id IS NULL;

-- 6. activities.recording_device_id setzen
UPDATE activities a SET recording_device_id = d.id
FROM devices d
WHERE d.type = 'recorder'
  AND d.user_id = a.user_id
  AND d.product_name = a.source_device
  AND a.recording_device_id IS NULL;
