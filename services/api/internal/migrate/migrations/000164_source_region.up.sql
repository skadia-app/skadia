-- 000164_source_region.up.sql

-- === osm_ways: KEIN UNIQUE(osm_id) — Duplikate erlaubt! ===
-- Lua-Script kann für gleiche osm_id mehrere Rows erzeugen (highway + ferry).
-- Border-Ways zwischen Regionen erzeugen ebenfalls Duplikate (~0.1%).
-- Junction-Table trackt welche Regionen einen Way "besitzen" — verhindert
-- Datenverlust beim Löschen (Border-Ways bleiben wenn andere Region sie hat).
-- Graph-Builder nutzt DISTINCT ON (osm_id) um parallele Kanten zu vermeiden.

CREATE TABLE osm_way_regions (
    osm_id BIGINT NOT NULL,
    region_id TEXT NOT NULL,
    PRIMARY KEY (osm_id, region_id)
);
CREATE INDEX idx_osm_way_regions_region ON osm_way_regions(region_id);

-- === osm_routes: KEIN UNIQUE(osm_id) — gleiche Logik wie osm_ways ===

CREATE TABLE osm_route_regions (
    osm_id BIGINT NOT NULL,
    region_id TEXT NOT NULL,
    PRIMARY KEY (osm_id, region_id)
);
CREATE INDEX idx_osm_route_regions_region ON osm_route_regions(region_id);

-- === Alle anderen Tabellen: source_region Spalte + Index ===
-- Kein UNIQUE — Duplikate an Grenzen erlaubt (harmlos, ~0.1%).
-- DELETE WHERE source_region = X entfernt sauber nur eine Region.
ALTER TABLE osm_pois ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_forests ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_boundaries ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_landuse ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_railways ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_waterways ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_waters ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_glaciers ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_wetlands ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_pistes ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_piste_areas ADD COLUMN IF NOT EXISTS source_region TEXT;
ALTER TABLE osm_lifts ADD COLUMN IF NOT EXISTS source_region TEXT;

CREATE INDEX IF NOT EXISTS idx_osm_pois_region ON osm_pois(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_forests_region ON osm_forests(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_boundaries_region ON osm_boundaries(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_landuse_region ON osm_landuse(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_railways_region ON osm_railways(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_waterways_region ON osm_waterways(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_waters_region ON osm_waters(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_glaciers_region ON osm_glaciers(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_wetlands_region ON osm_wetlands(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_pistes_region ON osm_pistes(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_piste_areas_region ON osm_piste_areas(source_region);
CREATE INDEX IF NOT EXISTS idx_osm_lifts_region ON osm_lifts(source_region);

-- === osm_import_regions: neue Spalten ===
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS replication_url TEXT;
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS last_sequence_number INT;
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS pbf_path TEXT;
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS update_interval TEXT DEFAULT 'manual';
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS update_weekday INT DEFAULT 1; -- 1=Montag (Go: time.Monday=1)
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS update_day INT DEFAULT 1;
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS update_hour INT DEFAULT 3;
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS data_date TIMESTAMPTZ;

-- === Backfill: bestehende Daten mit source_region versehen ===
DO $$
DECLARE
  _region TEXT;
  _count INT;
BEGIN
  SELECT count(*), min(region_id) INTO _count, _region
  FROM osm_import_regions WHERE status = 'imported';
  IF _count >= 1 AND _region IS NOT NULL THEN
    -- Alle bestehenden Rows der (einzigen) importierten Region zuordnen
    UPDATE osm_pois SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_forests SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_boundaries SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_landuse SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_railways SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_waterways SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_waters SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_glaciers SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_wetlands SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_pistes SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_piste_areas SET source_region = _region WHERE source_region IS NULL;
    UPDATE osm_lifts SET source_region = _region WHERE source_region IS NULL;
    -- Junction-Tables für osm_ways + osm_routes
    INSERT INTO osm_way_regions (osm_id, region_id)
    SELECT osm_id, _region FROM osm_ways WHERE highway IS NOT NULL
    ON CONFLICT DO NOTHING;
    INSERT INTO osm_route_regions (osm_id, region_id)
    SELECT osm_id, _region FROM osm_routes
    ON CONFLICT DO NOTHING;
  END IF;
END $$;

-- === GRANTs ===
GRANT SELECT ON osm_way_regions TO skadia_readonly;
GRANT SELECT ON osm_route_regions TO skadia_readonly;

-- === Cleanup: enrichment.raster_tiles (nicht mehr verwendet seit Performance Overhaul) ===
DROP TABLE IF EXISTS enrichment.raster_tiles;
DROP SCHEMA IF EXISTS enrichment;
