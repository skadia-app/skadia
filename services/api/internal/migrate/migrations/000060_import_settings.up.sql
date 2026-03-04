-- Activity Fusion columns
ALTER TABLE activities
  ADD COLUMN IF NOT EXISTS fusion_sources JSONB,
  ADD COLUMN IF NOT EXISTS is_fusion_source BOOLEAN NOT NULL DEFAULT FALSE;

-- Import settings on users table
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS import_conflict_strategy TEXT NOT NULL DEFAULT 'enrich_missing'
    CHECK (import_conflict_strategy IN ('enrich_missing', 'replace_all', 'skip')),
  ADD COLUMN IF NOT EXISTS gps_duplicate_threshold INT NOT NULL DEFAULT 75;
-- percentage 0–100: minimum GPS-track overlap to suggest fusion candidate

-- For gear duplicate detection (Levenshtein distance)
CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;
