ALTER TABLE system_flags
    DROP CONSTRAINT IF EXISTS system_flags_expires_at_check;

ALTER TABLE system_flags
    DROP CONSTRAINT IF EXISTS system_flags_check;

ALTER TABLE system_flags
    ADD CONSTRAINT system_flags_expires_at_check
    CHECK (key = 'maintenance_mode' OR expires_at <= set_at + INTERVAL '48 hours');
