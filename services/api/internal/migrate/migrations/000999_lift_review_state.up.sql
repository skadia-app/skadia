-- Phase 4d — Soft-Reject-Persistenz + Acknowledgment-State.
--
-- rejected_by_user: soft-reject flag on lift candidates. Hidden from default
-- queries and aggregates. Survives re-enrichment unless explicitly cleared via
-- /lifts/recalculate-all. User-authored data — not regenerable on re-import.
--
-- idx_lift_candidates_active: partial index for the common query path (active
-- = not rejected). Must be dropped before rejected_by_user is dropped.
--
-- lifts_acknowledged_at: set when the user has reviewed lift candidates
-- (confirm-all / reject-all / single-action via review modal). NULL means the
-- modal should show on the next visit.
--
-- ADD COLUMN ... DEFAULT FALSE in Postgres 11+ is a catalog-only operation
-- (no table rewrite) when the column is NOT NULL with a constant default.

ALTER TABLE activity_lift_candidates
    ADD COLUMN IF NOT EXISTS rejected_by_user BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_lift_candidates_active
    ON activity_lift_candidates (activity_id)
    WHERE NOT rejected_by_user;

ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS lifts_acknowledged_at TIMESTAMPTZ;

COMMENT ON COLUMN activity_lift_candidates.rejected_by_user IS
    'Soft-reject flag. Hidden from default queries / aggregates. Survives re-enrichment unless explicitly cleared via /lifts/recalculate-all.';

COMMENT ON COLUMN activities.lifts_acknowledged_at IS
    'Set when the user has reviewed lift candidates (confirm-all/reject-all/single-action via review modal). NULL means modal should show on next visit.';
