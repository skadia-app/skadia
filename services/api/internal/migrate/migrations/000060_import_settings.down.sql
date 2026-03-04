ALTER TABLE activities
  DROP COLUMN IF EXISTS fusion_sources,
  DROP COLUMN IF EXISTS is_fusion_source;

ALTER TABLE users
  DROP COLUMN IF EXISTS import_conflict_strategy,
  DROP COLUMN IF EXISTS gps_duplicate_threshold;

-- fuzzystrmatch extension is intentionally not dropped (shared resource, may be used by other schemas)
