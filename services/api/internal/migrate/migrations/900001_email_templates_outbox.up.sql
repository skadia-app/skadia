CREATE TABLE email_templates (
    key       TEXT PRIMARY KEY,
    subject   TEXT NOT NULL,
    html_body TEXT NOT NULL,
    text_body TEXT NOT NULL,
    enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE email_outbox (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID REFERENCES users(id) ON DELETE CASCADE,
    template_key    TEXT NOT NULL,
    recipient       TEXT NOT NULL DEFAULT '',
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    recipient_ciphertext TEXT NOT NULL,
    payload_ciphertext   TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'sent', 'failed')),
    attempt_count   INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    max_attempts    INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT now() + interval '24 hours',
    claimed_by      TEXT,
    claimed_at      TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX email_outbox_pending_idx
    ON email_outbox (next_attempt_at, expires_at, created_at)
    WHERE status = 'pending';

CREATE INDEX email_outbox_user_idx
    ON email_outbox (user_id)
    WHERE user_id IS NOT NULL;

CREATE INDEX email_outbox_status_created_idx
    ON email_outbox (status, created_at DESC);

CREATE TABLE email_rate_limits (
    key     TEXT PRIMARY KEY,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
