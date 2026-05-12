ALTER TABLE automation_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE automation_rules FORCE ROW LEVEL SECURITY;

GRANT SELECT ON automation_rules TO skadia_readonly;

CREATE POLICY automation_rules_readonly_select ON automation_rules
    FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id', true)::uuid);

CREATE POLICY automation_rules_owner_all ON automation_rules
    FOR ALL TO PUBLIC
    USING (user_id = current_setting('app.current_user_id', true)::uuid)
    WITH CHECK (user_id = current_setting('app.current_user_id', true)::uuid);
