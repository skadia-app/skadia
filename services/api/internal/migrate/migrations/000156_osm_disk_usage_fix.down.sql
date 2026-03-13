-- Restore original view with schemaname
DROP VIEW IF EXISTS osm_disk_usage;

CREATE VIEW osm_disk_usage AS
SELECT
    schemaname,
    tablename,
    pg_total_relation_size(schemaname || '.' || tablename) AS total_bytes
FROM pg_tables
WHERE tablename LIKE 'osm_%'
  AND schemaname = 'public'
ORDER BY total_bytes DESC;

GRANT SELECT ON osm_disk_usage TO skadia_readonly;
