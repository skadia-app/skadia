-- osm_ways.way_id braucht eine eigene Sequence für Versionierung bei
-- inkrementellen Region-Updates. Der Router nutzt DISTINCT ON (osm_id)
-- ORDER BY osm_id, way_id DESC — höchster way_id = neuester Import.
--
-- osm2pgsql füllt way_id im Staging-Schema mit der OSM Way-ID (deterministisch).
-- importRegionFromStaging schließt way_id bewusst aus, damit public einen
-- eigenen sequenziellen Wert generiert. Ohne Sequence → NULL → Constraint-Fehler.
--
-- NUR osm_ways braucht das — alle anderen Tabellen nutzen source_region
-- für Region-Management und kopieren ihre IDs direkt aus dem Staging.
CREATE SEQUENCE IF NOT EXISTS osm_ways_way_id_seq;
SELECT setval('osm_ways_way_id_seq', COALESCE((SELECT MAX(way_id) FROM osm_ways), 0) + 1, false);
ALTER TABLE osm_ways ALTER COLUMN way_id SET DEFAULT nextval('osm_ways_way_id_seq');
ALTER SEQUENCE osm_ways_way_id_seq OWNED BY osm_ways.way_id;
