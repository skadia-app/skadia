UPDATE system_flags
SET expires_at = set_at + INTERVAL '48 hours'
WHERE expires_at > set_at + INTERVAL '48 hours';

ALTER TABLE system_flags
    DROP CONSTRAINT IF EXISTS system_flags_expires_at_check;

ALTER TABLE system_flags
    DROP CONSTRAINT IF EXISTS system_flags_check;

ALTER TABLE system_flags
    ADD CONSTRAINT system_flags_expires_at_check
    CHECK (expires_at <= set_at + INTERVAL '48 hours');
