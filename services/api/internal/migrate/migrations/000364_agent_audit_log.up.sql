CREATE TABLE agent_audit_log (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    session_id    UUID NOT NULL,
    tool_call_id  TEXT NOT NULL,
    tool_name     TEXT NOT NULL,
    args          JSONB NOT NULL,
    result_status TEXT NOT NULL CHECK (result_status IN ('pending','executed','declined','timeout','error')),
    result_detail JSONB,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at  TIMESTAMPTZ,
    CONSTRAINT agent_audit_uniq UNIQUE (user_id, session_id, tool_call_id)
);

CREATE INDEX agent_audit_user_time    ON agent_audit_log (user_id, requested_at DESC);
CREATE INDEX agent_audit_tool         ON agent_audit_log (tool_name, requested_at DESC);
CREATE INDEX agent_audit_session_time ON agent_audit_log (session_id, requested_at DESC);

ALTER TABLE agent_audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_audit_log FORCE ROW LEVEL SECURITY;

CREATE POLICY agent_audit_owner_rw ON agent_audit_log
    FOR ALL TO PUBLIC
    USING (user_id = current_setting('app.current_user_id', true)::uuid)
    WITH CHECK (user_id = current_setting('app.current_user_id', true)::uuid);

CREATE POLICY agent_audit_purge_anonymize ON agent_audit_log
    FOR UPDATE TO PUBLIC
    USING (user_id = current_setting('app.current_user_id', true)::uuid)
    WITH CHECK (user_id IS NULL);
