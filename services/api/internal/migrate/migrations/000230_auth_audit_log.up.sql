CREATE TABLE auth_audit_log (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    event_type  TEXT NOT NULL,
    ip_address  INET,
    user_agent  TEXT,
    details     JSONB,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_auth_audit_log_user ON auth_audit_log(user_id);
CREATE INDEX idx_auth_audit_log_type ON auth_audit_log(event_type, created_at);
CREATE INDEX idx_auth_audit_log_created ON auth_audit_log(created_at);
