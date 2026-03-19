-- 000192: Consolidate enrichment settings — remove Stage-1/Stage-2 split,
-- standardize on _detection suffix.
-- NOTE: String-literal module names ("peaks", "lifts" etc.) in Go/NATS are NOT changed.

-- 1) Merge duplicate fields: AND both values into the surviving column
UPDATE user_enrichment_settings SET
  enable_summit_detection  = enable_summit_detection  AND COALESCE(enable_peaks, true),
  enable_stop_detection    = enable_stop_detection    AND COALESCE(enable_stops, true),
  enable_commute_detection = enable_commute_detection AND COALESCE(enable_commute, true);

-- 2) Rename enable_climb_detection -> enable_ascent_detection + merge enable_ascents
ALTER TABLE user_enrichment_settings
  RENAME COLUMN enable_climb_detection TO enable_ascent_detection;
UPDATE user_enrichment_settings SET
  enable_ascent_detection = enable_ascent_detection AND COALESCE(enable_ascents, true);

-- 3) Drop merged Stage-2 duplicate columns
ALTER TABLE user_enrichment_settings
  DROP COLUMN IF EXISTS enable_peaks,
  DROP COLUMN IF EXISTS enable_stops,
  DROP COLUMN IF EXISTS enable_commute,
  DROP COLUMN IF EXISTS enable_ascents;

-- 4) Rename Stage-2-only columns to _detection suffix
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_forests     TO enable_forest_detection;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_water       TO enable_water_detection;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_settlements TO enable_settlement_detection;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_borders     TO enable_border_detection;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_lifts       TO enable_lift_detection;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_surfaces    TO enable_surface_detection;
ALTER TABLE user_enrichment_settings RENAME COLUMN enable_intervals   TO enable_interval_detection;
