CREATE TABLE server_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    encrypted   BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  UUID REFERENCES users(id) ON DELETE SET NULL
);

COMMENT ON TABLE server_settings IS 'Instance-wide server configuration (SMTP, limits, etc). Values with encrypted=true are AES-256-GCM encrypted.';

-- Seed TimescaleDB compression state from current DB state.
-- Existing installs with compression (000079) get 'true', new installs get 'false'.
INSERT INTO server_settings (key, value, encrypted, updated_at)
SELECT 'timescale.compression',
       CASE WHEN count(*) > 0 THEN 'true' ELSE 'false' END,
       false,
       now()
FROM timescaledb_information.compression_settings
WHERE hypertable_name = 'activity_streams';
