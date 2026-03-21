-- 000202_device_data_repair.up.sql
-- Repariert devices-Daten: Shadow Devices eliminieren + product_name/manufacturer bereinigen.

-- 1. product_name reparieren für Devices mit GarminProductInvalid-Strings.
-- Wenn ein anderes Device mit gleicher serial_number einen besseren product_name hat, übernehmen.
UPDATE devices d
SET product_name = better.product_name
FROM devices better
WHERE d.product_name LIKE 'GarminProduct%'
  AND better.serial_number IS NOT NULL
  AND d.serial_number IS NOT NULL
  AND better.serial_number = d.serial_number
  AND better.user_id = d.user_id
  AND better.type = d.type
  AND better.id != d.id
  AND better.product_name IS NOT NULL
  AND better.product_name NOT LIKE 'GarminProduct%'
  AND length(better.product_name) > 0;

-- 2. Shadow Devices (serial_number IS NULL) deren activity_sensors an Canonical Devices
-- (serial_number IS NOT NULL, gleicher device_type) umhängen.
-- Nur wenn GENAU EIN Canonical Device matched (verhindert non-deterministische Zuordnung).
UPDATE activity_sensors s
SET device_id = match.canonical_id
FROM (
    SELECT shadow.id AS shadow_id, MIN(canonical.id) AS canonical_id
    FROM devices shadow
    JOIN devices canonical
      ON canonical.user_id = shadow.user_id
      AND canonical.type = shadow.type
      AND canonical.serial_number IS NOT NULL
      AND canonical.device_types && shadow.device_types
    WHERE shadow.serial_number IS NULL
      AND shadow.type = 'sensor'
    GROUP BY shadow.id
    HAVING COUNT(DISTINCT canonical.id) = 1
) match
WHERE s.device_id = match.shadow_id;

-- 2b: Recorder Shadow Devices analog umhängen
UPDATE activities a
SET recording_device_id = canonical.id
FROM devices shadow
JOIN devices canonical
  ON canonical.user_id = shadow.user_id
  AND canonical.type = shadow.type
  AND canonical.serial_number IS NOT NULL
  AND shadow.serial_number IS NULL
  AND canonical.product_name = shadow.product_name
WHERE a.recording_device_id = shadow.id
  AND shadow.type = 'recorder';

-- 3. Verwaiste Shadow Devices löschen (keine activity_sensors/activities mehr verknüpft)
DELETE FROM devices d
WHERE d.serial_number IS NULL
  AND NOT EXISTS (SELECT 1 FROM activity_sensors s WHERE s.device_id = d.id)
  AND NOT EXISTS (SELECT 1 FROM activities a WHERE a.recording_device_id = d.id);

-- 4. ManufacturerInvalid-Strings in devices.manufacturer bereinigen
UPDATE devices
SET manufacturer = NULL
WHERE manufacturer LIKE 'ManufacturerInvalid%'
   OR manufacturer LIKE 'Manufacturer(%';

-- 5. Noch verbleibende GarminProductInvalid product_names leeren
-- (kein besseres Device gefunden → besser NULL als falscher String)
UPDATE devices
SET product_name = NULL
WHERE product_name LIKE 'GarminProduct%';

-- 6. ManufacturerInvalid-Strings in activity_sensors.manufacturer bereinigen
UPDATE activity_sensors
SET manufacturer = NULL
WHERE manufacturer LIKE 'ManufacturerInvalid%'
   OR manufacturer LIKE 'Manufacturer(%';
