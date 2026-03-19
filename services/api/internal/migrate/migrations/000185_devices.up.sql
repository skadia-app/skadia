-- 000185_devices.up.sql

CREATE TABLE devices (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type              TEXT NOT NULL CHECK (type IN ('recorder', 'sensor')),
    manufacturer      TEXT,
    manufacturer_id   INTEGER,
    product_name      TEXT,
    product_id        INTEGER,
    serial_number     BIGINT,
    ant_device_number INTEGER,
    custom_name       TEXT,
    device_types      TEXT[],
    source_type       TEXT,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CHECK (type = 'recorder' OR source_type IS NULL OR source_type IN ('antplus', 'bluetooth'))
);

CREATE UNIQUE INDEX idx_devices_serial
    ON devices (user_id, type, manufacturer_id, serial_number)
    WHERE serial_number IS NOT NULL;

CREATE UNIQUE INDEX idx_devices_ant
    ON devices (user_id, type, manufacturer_id, ant_device_number)
    WHERE ant_device_number IS NOT NULL AND serial_number IS NULL;

CREATE UNIQUE INDEX idx_devices_name
    ON devices (user_id, type, COALESCE(manufacturer_id, 0), product_name)
    WHERE serial_number IS NULL AND ant_device_number IS NULL;

CREATE INDEX idx_devices_user ON devices (user_id);

ALTER TABLE activities ADD COLUMN IF NOT EXISTS recording_device_id UUID REFERENCES devices(id) ON DELETE SET NULL;

ALTER TABLE activity_sensors ADD COLUMN IF NOT EXISTS device_id UUID REFERENCES devices(id) ON DELETE SET NULL;
