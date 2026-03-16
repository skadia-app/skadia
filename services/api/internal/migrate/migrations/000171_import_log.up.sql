CREATE TABLE import_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_platform TEXT NOT NULL,
    filename        TEXT NOT NULL,
    file_hash       TEXT,
    file_size_bytes BIGINT,
    status          TEXT NOT NULL DEFAULT 'pending',
    activity_id     UUID REFERENCES activities(id) ON DELETE SET NULL,
    error_msg       TEXT,
    duration_ms     INT,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ON import_log (user_id, created_at DESC);
CREATE INDEX ON import_log (user_id, status) WHERE status IN ('pending', 'processing');
CREATE UNIQUE INDEX ON import_log (user_id, file_hash) WHERE status IN ('pending', 'processing');
