-- Region-Tracking für OSM-Importe
CREATE TABLE IF NOT EXISTS osm_import_regions (
    region_id       TEXT PRIMARY KEY,
    label           TEXT NOT NULL,
    continent       TEXT NOT NULL,
    imported_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    pbf_date        TEXT,
    pbf_size_bytes  BIGINT,
    import_duration_s INTEGER,
    categories      TEXT[] NOT NULL DEFAULT '{}',
    status          TEXT NOT NULL DEFAULT 'imported'
        CHECK (status IN ('importing', 'imported', 'failed'))
);

-- Disk-Usage-View (geschätzte Größe der OSM-Tabellen)
CREATE OR REPLACE VIEW osm_disk_usage AS
SELECT
    schemaname,
    tablename,
    pg_total_relation_size(schemaname || '.' || tablename) AS total_bytes
FROM pg_tables
WHERE tablename LIKE 'osm_%'
  AND schemaname = 'public'
ORDER BY total_bytes DESC;

GRANT SELECT ON osm_import_regions TO skadia_readonly;
GRANT SELECT ON osm_disk_usage TO skadia_readonly;
