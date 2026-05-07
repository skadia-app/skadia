-- Phase 4d rollback — drop in dependency order:
-- index first (depends on rejected_by_user column), then the columns.
-- Run grep first: grep -r "rejected_by_user\|lifts_acknowledged_at" services/api/ frontend/src/
-- must be empty (global CLAUDE.md "Database column drops" rule).

DROP INDEX IF EXISTS idx_lift_candidates_active;
ALTER TABLE activity_lift_candidates DROP COLUMN IF EXISTS rejected_by_user;
ALTER TABLE activities DROP COLUMN IF EXISTS lifts_acknowledged_at;
