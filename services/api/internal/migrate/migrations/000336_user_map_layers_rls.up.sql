-- 000336_user_map_layers_rls.up.sql
-- Defense-in-depth: enable Row-Level Security on user_map_layers.
--
-- Mirrors the pattern established in migration 000111_skadia_readonly:
--   * Policies are scoped FOR SELECT TO skadia_readonly (the SQL Agent role).
--   * The session variable app.current_user_id is set per-request by the
--     SQL Agent before each query (see services/ml/agent/sql_agent.py).
--   * The owner role (skadia) is unaffected — RLS does not apply to table
--     owners by default, so the main API continues to operate unchanged.
--
-- user_map_layers is currently NOT in the skadia_readonly GRANT whitelist
-- (mig 000111:16-23). Enabling RLS now means: should the table ever be added
-- to that whitelist in the future, user-isolation is already enforced at the
-- DB layer. This is the defense-in-depth fence the project's RLS pattern
-- exists to provide.

ALTER TABLE user_map_layers ENABLE ROW LEVEL SECURITY;

CREATE POLICY user_isolation_direct ON user_map_layers FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
