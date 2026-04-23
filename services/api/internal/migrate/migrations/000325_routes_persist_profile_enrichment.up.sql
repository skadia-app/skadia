ALTER TABLE planned_routes
  ADD COLUMN elevation_profile JSONB,
  ADD COLUMN enrichment        JSONB;

COMMENT ON COLUMN planned_routes.elevation_profile IS
  'Full-resolution elevation profile as [{dist_m, ele_m}, ...] or NULL when DEM unavailable at save time';
COMMENT ON COLUMN planned_routes.enrichment IS
  'Derived route stats (version, forest_pct, sac_ranges, summits_passed, ...) or NULL';
