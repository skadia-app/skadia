DROP POLICY IF EXISTS automation_rules_readonly_select ON automation_rules;
DROP POLICY IF EXISTS automation_rules_owner_all ON automation_rules;
REVOKE SELECT ON automation_rules FROM skadia_readonly;
ALTER TABLE IF EXISTS automation_rules NO FORCE ROW LEVEL SECURITY;
ALTER TABLE IF EXISTS automation_rules DISABLE ROW LEVEL SECURITY;
