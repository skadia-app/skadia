-- Phase 4d Task 39 — add dismissed_at to fusion_candidates.
-- Enables the workshop "nicht fusionieren"-dismiss flow: user rejects a
-- candidate pair; it stops appearing in the fusion queue.
BEGIN;
ALTER TABLE fusion_candidates ADD COLUMN IF NOT EXISTS dismissed_at TIMESTAMPTZ NULL;
CREATE INDEX IF NOT EXISTS fusion_candidates_dismissed_at_idx
  ON fusion_candidates (dismissed_at) WHERE dismissed_at IS NOT NULL;
COMMIT;
