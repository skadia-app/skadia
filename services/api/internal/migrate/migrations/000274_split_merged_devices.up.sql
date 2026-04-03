-- 000274_split_merged_devices.up.sql
--
-- Splits "merged" recorder devices that were created with only the manufacturer
-- name as product_name (the fallback when ResolveProductName couldn't identify
-- the model). For each such device, activities whose source_device differs from
-- the manufacturer name are moved to dedicated per-model device records.
--
-- Root cause: parser.go passed "" to ResolveProductName instead of the device
-- name extracted from the FIT file. Fixed in parser.go simultaneously.

DO $$
DECLARE
    old_dev RECORD;
    src     RECORD;
    new_id  UUID;
BEGIN
    FOR old_dev IN
        SELECT id, user_id, type, manufacturer, manufacturer_id,
               source_type, product_id
        FROM devices
        WHERE product_name = manufacturer
          AND serial_number IS NULL
          AND ant_device_number IS NULL
          AND type = 'recorder'
    LOOP
        FOR src IN
            SELECT
                a.source_device,
                MIN(a.started_at) AS first_seen,
                MAX(a.started_at) AS last_seen
            FROM activities a
            WHERE a.recording_device_id = old_dev.id
              AND a.source_device IS NOT NULL
              AND a.source_device != ''
              AND a.source_device != old_dev.manufacturer
            GROUP BY a.source_device
        LOOP
            -- Insert new specific device (ignore conflict if already exists)
            INSERT INTO devices (
                user_id, type, manufacturer, manufacturer_id,
                product_name, product_id, source_type,
                first_seen_at, last_seen_at, created_at
            )
            VALUES (
                old_dev.user_id, old_dev.type,
                old_dev.manufacturer, old_dev.manufacturer_id,
                src.source_device, 0, old_dev.source_type,
                src.first_seen, src.last_seen, NOW()
            )
            ON CONFLICT (user_id, type, COALESCE(manufacturer_id, 0), product_name)
                WHERE serial_number IS NULL AND ant_device_number IS NULL
                DO NOTHING;

            SELECT id INTO new_id
            FROM devices
            WHERE user_id = old_dev.user_id
              AND type = old_dev.type
              AND COALESCE(manufacturer_id, 0) = COALESCE(old_dev.manufacturer_id, 0)
              AND product_name = src.source_device
              AND serial_number IS NULL
              AND ant_device_number IS NULL
            ORDER BY id
            LIMIT 1;

            IF new_id IS NOT NULL THEN
                -- Relink activities that have this specific source_device
                UPDATE activities
                SET recording_device_id = new_id
                WHERE recording_device_id = old_dev.id
                  AND source_device = src.source_device;

                -- Keep first_seen/last_seen on new device accurate
                UPDATE devices
                SET first_seen_at = LEAST(first_seen_at, src.first_seen),
                    last_seen_at  = GREATEST(last_seen_at, src.last_seen)
                WHERE id = new_id;
            END IF;
        END LOOP;
    END LOOP;

    -- Remove merged devices that lost all their activities to the split
    DELETE FROM devices d
    WHERE d.product_name = d.manufacturer
      AND d.serial_number IS NULL
      AND d.ant_device_number IS NULL
      AND d.type = 'recorder'
      AND NOT EXISTS (
          SELECT 1 FROM activities a WHERE a.recording_device_id = d.id
      );
END $$;
