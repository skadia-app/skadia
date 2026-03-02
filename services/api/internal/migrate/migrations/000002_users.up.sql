CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        TEXT UNIQUE NOT NULL,
    display_name    TEXT,
    email           TEXT UNIQUE,
    units           TEXT NOT NULL DEFAULT 'metric',
    locale          TEXT NOT NULL DEFAULT 'de',
    is_admin        BOOLEAN DEFAULT false,
    is_active       BOOLEAN DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Dev user for local testing.
INSERT INTO users (id, username, display_name, email, is_admin)
VALUES ('00000000-0000-0000-0000-000000000001', 'dev', 'Dev User', 'dev@localhost', true);
