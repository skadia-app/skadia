-- NOTE: AND-merged values cannot be perfectly restored to their original
-- independent Stage-1/Stage-2 state. The down migration copies the merged
-- value into both the re-created columns, which is the best approximation.

-- Reverse renames
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_forest_detection     TO enable_forests;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_water_detection      TO enable_water;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_settlement_detection TO enable_settlements;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_border_detection     TO enable_borders;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_lift_detection       TO enable_lifts;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_surface_detection    TO enable_surfaces;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_interval_detection   TO enable_intervals;

-- Reverse ascent rename
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_ascent_detection TO enable_climb_detection;

-- Re-add dropped columns with defaults
ALTER TABLE user_enrichment_settings
  ADD COLUMN enable_peaks   BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN enable_stops   BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN enable_commute BOOLEAN NOT NULL DEFAULT true,
  ADD COLUMN enable_ascents BOOLEAN NOT NULL DEFAULT true;

-- Copy merged values back (best approximation)
UPDATE user_enrichment_settings SET
  enable_peaks   = enable_summit_detection,
  enable_stops   = enable_stop_detection,
  enable_commute = enable_commute_detection,
  enable_ascents = enable_climb_detection;
