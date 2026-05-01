-- 000344_rls_user_readiness_preferences.down.sql
-- Reverts RLS on user_readiness_preferences (mirrors 000336_user_map_layers_rls.down.sql shape).

DROP POLICY IF EXISTS user_isolation_direct ON user_readiness_preferences;
ALTER TABLE user_readiness_preferences DISABLE ROW LEVEL SECURITY;
