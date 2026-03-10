-- Migration 000126: Grant SELECT on analytics_cache to skadia_readonly role
-- The ML SQL Agent needs read access to analytics_cache to answer analytics questions.
-- This table was added after the initial grant block in 000111_skadia_readonly.

GRANT SELECT ON analytics_cache TO skadia_readonly;
