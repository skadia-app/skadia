-- 000281_osm_needs_redownload.up.sql

-- Erweitere Status-CHECK um 'needs_redownload'
ALTER TABLE osm_import_regions DROP CONSTRAINT IF EXISTS osm_import_regions_status_check;
ALTER TABLE osm_import_regions ADD CONSTRAINT osm_import_regions_status_check
    CHECK (status IN ('importing', 'imported', 'failed', 'needs_redownload'));

-- Letzte erfolgreiche Aktualisierung (Patch oder Import)
ALTER TABLE osm_import_regions ADD COLUMN IF NOT EXISTS last_updated_at TIMESTAMPTZ;

-- Initialisierung: last_updated_at = imported_at für bestehende Regionen
UPDATE osm_import_regions SET last_updated_at = imported_at WHERE last_updated_at IS NULL;
