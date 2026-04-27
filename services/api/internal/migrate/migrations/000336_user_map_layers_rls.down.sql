-- 000336_user_map_layers_rls.down.sql
-- Reverts RLS on user_map_layers (mirrors 000111_skadia_readonly.down.sql shape).

DROP POLICY IF EXISTS user_isolation_direct ON user_map_layers;
ALTER TABLE user_map_layers DISABLE ROW LEVEL SECURITY;
