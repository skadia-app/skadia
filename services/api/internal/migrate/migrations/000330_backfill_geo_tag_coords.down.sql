-- 000330_backfill_geo_tag_coords.down.sql
-- Reverting this migration would clear lat/lng from point-type geo tags.
-- That would break map marker rendering again; only roll back if the
-- corresponding code fix in geo_enrich.go / stops.go is also reverted.

UPDATE activity_geo_tags
SET lat = NULL,
    lng = NULL
WHERE category IN ('summit', 'pass', 'settlement', 'gastronomy')
  AND lat IS NOT NULL;
