-- Migration 000126 DOWN: Revoke SELECT on analytics_cache from skadia_readonly role

REVOKE SELECT ON analytics_cache FROM skadia_readonly;
