CREATE TABLE IF NOT EXISTS system_flags (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    set_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
    CHECK (expires_at <= set_at + INTERVAL '48 hours')
);
