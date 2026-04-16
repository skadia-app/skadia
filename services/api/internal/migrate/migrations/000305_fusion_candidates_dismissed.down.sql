BEGIN;
DROP INDEX IF EXISTS fusion_candidates_dismissed_at_idx;
ALTER TABLE fusion_candidates DROP COLUMN IF EXISTS dismissed_at;
COMMIT;
