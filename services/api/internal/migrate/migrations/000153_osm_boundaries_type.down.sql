DROP INDEX IF EXISTS idx_osm_boundaries_type;
ALTER TABLE osm_boundaries DROP COLUMN IF EXISTS protect_class;
ALTER TABLE osm_boundaries DROP COLUMN IF EXISTS boundary_type;
