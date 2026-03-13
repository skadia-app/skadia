-- Fix osm_disk_usage view: remove schemaname column (not used by API)
-- DROP required because PostgreSQL cannot remove columns from an existing view via CREATE OR REPLACE
DROP VIEW IF EXISTS osm_disk_usage;

CREATE VIEW osm_disk_usage AS
SELECT
    tablename,
    pg_total_relation_size('public.' || tablename) AS total_bytes
FROM pg_tables
WHERE tablename LIKE 'osm_%'
  AND schemaname = 'public'
ORDER BY total_bytes DESC;

GRANT SELECT ON osm_disk_usage TO skadia_readonly;
