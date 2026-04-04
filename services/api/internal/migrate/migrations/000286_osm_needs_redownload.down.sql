-- 000281_osm_needs_redownload.down.sql

-- Setze needs_redownload Regionen auf 'failed' zurück
UPDATE osm_import_regions SET status = 'failed' WHERE status = 'needs_redownload';

ALTER TABLE osm_import_regions DROP CONSTRAINT IF EXISTS osm_import_regions_status_check;
ALTER TABLE osm_import_regions ADD CONSTRAINT osm_import_regions_status_check
    CHECK (status IN ('importing', 'imported', 'failed'));

ALTER TABLE osm_import_regions DROP COLUMN IF EXISTS last_updated_at;
