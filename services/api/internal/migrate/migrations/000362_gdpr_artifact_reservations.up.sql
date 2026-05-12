CREATE TABLE IF NOT EXISTS gdpr_artifact_reservations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL REFERENCES background_jobs(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    backend         TEXT NOT NULL CHECK (backend IN ('local', 's3')),
    kind            TEXT NOT NULL CHECK (kind IN ('export', 'import')),
    reserved_bytes  BIGINT NOT NULL CHECK (reserved_bytes >= 0),
    committed_bytes BIGINT NOT NULL DEFAULT 0 CHECK (committed_bytes >= 0),
    expires_at      TIMESTAMPTZ NOT NULL,
    released_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_id)
);

CREATE INDEX IF NOT EXISTS idx_gdpr_artifact_reservations_user_active
    ON gdpr_artifact_reservations (user_id)
    WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_gdpr_artifact_reservations_backend_active
    ON gdpr_artifact_reservations (backend)
    WHERE released_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_gdpr_artifact_reservations_expiry_active
    ON gdpr_artifact_reservations (expires_at)
    WHERE released_at IS NULL;
