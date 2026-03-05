-- 000080_drop_planet_osm.up.sql
-- Loescht die osm2pgsql Slim-Modus Hilfstabellen (planet_osm_nodes,
-- planet_osm_ways, planet_osm_rels).
--
-- Diese Tabellen werden ausschliesslich waehrend des osm2pgsql-Imports
-- benoetigt, um Way-zu-Node-Referenzen aufzuloesen. Nach dem Import
-- sind alle relevanten Geodaten in die osm_*-Tabellen extrahiert:
--   osm_waterways, osm_waters, osm_peaks, osm_forests,
--   osm_amenities, osm_lifts, osm_settlements, osm_boundaries
--
-- Weder Martin (Tile-Server), noch der Enrichment-Service, noch die
-- Go-API referenzieren planet_osm_*-Tabellen.
--
-- Einsparung: ~61 GB
--   - planet_osm_nodes: 38 GB (569 Mio. Knoten + 12 GB B-Tree-Index)
--   - planet_osm_ways:  22 GB
--   - planet_osm_rels:   1.4 GB
--
-- IRREVERSIBEL: Wiederherstellung nur via OSM-Re-Import:
--   bash scripts/import-osm.sh [--region dach]
--   (ohne --drop Flag)

DROP TABLE IF EXISTS planet_osm_nodes CASCADE;
DROP TABLE IF EXISTS planet_osm_ways CASCADE;
DROP TABLE IF EXISTS planet_osm_rels CASCADE;
