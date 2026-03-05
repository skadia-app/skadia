-- 000080_drop_planet_osm.down.sql
-- Diese Migration ist IRREVERSIBEL.
-- Die planet_osm_*-Tabellen (61 GB OSM-Rohdaten) koennen nicht aus
-- der Datenbank rekonstruiert werden.
--
-- Wiederherstellung: bash scripts/import-osm.sh [--region dach]
-- (ohne --drop Flag in osm2pgsql)

-- Intentionally empty: irreversible migration.
SELECT 'planet_osm_* tables (61 GB) were permanently dropped in migration 000080. '
    || 'To restore: bash scripts/import-osm.sh [--region dach]'
    AS restore_info;
