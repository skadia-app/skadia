ALTER TABLE planned_routes
  DROP COLUMN IF EXISTS elevation_profile,
  DROP COLUMN IF EXISTS enrichment;
