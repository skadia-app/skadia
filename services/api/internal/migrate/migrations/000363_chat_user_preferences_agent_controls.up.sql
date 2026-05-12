CREATE TABLE chat_user_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    chat_expose_document_extracted_data BOOLEAN NOT NULL DEFAULT false,
    chat_write_tools_enabled BOOLEAN NOT NULL DEFAULT false,
    chat_allowed_write_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chat_user_preferences_allowed_write_tools_array
        CHECK (jsonb_typeof(chat_allowed_write_tools) = 'array')
);

ALTER TABLE chat_user_preferences ENABLE ROW LEVEL SECURITY;
ALTER TABLE chat_user_preferences FORCE ROW LEVEL SECURITY;

GRANT SELECT ON chat_user_preferences TO skadia_readonly;

CREATE POLICY cup_read ON chat_user_preferences
    FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id', true)::uuid);

CREATE POLICY cup_rw ON chat_user_preferences
    FOR ALL TO PUBLIC
    USING (user_id = current_setting('app.current_user_id', true)::uuid)
    WITH CHECK (user_id = current_setting('app.current_user_id', true)::uuid);
