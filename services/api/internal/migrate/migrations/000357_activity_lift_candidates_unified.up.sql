-- Phase 4c — OSM-Snap-Pipeline: A/B-Doppel-Erkennung eliminieren.
--
-- Adds the source/segment_effort_id columns on activity_lift_candidates so
-- that Stage 1 + 2 + 3 of the snap pipeline can mark heuristic candidates
-- as redundant when the same physical ride already produced a segment_efforts
-- row, and adds three boolean audit flags on segment_efforts so each row
-- carries the stage that produced it (Stage 1 conservative snap, Stage 2
-- cable-hold merge, Stage 3 method-A coverage gap).
--
-- Type choice for segment_effort_id: segment_efforts has a composite PK
-- (id UUID, started_at TIMESTAMPTZ) due to the TimescaleDB hypertable. A
-- single-column FK cannot reference a composite PK in Postgres; the type is
-- therefore UUID without a FOREIGN KEY constraint, and the writing code
-- (services/api/internal/enrichment/lifts/snap.go) is responsible for
-- maintaining the link. The dedup contract is encoded in
-- recomputeLiftStats: heuristic candidates with segment_effort_id IS NOT NULL
-- are excluded from the heuristic-only count.
--
-- All statements are IF NOT EXISTS / IF EXISTS so re-running the migration
-- against a partially-applied DB is a no-op.

ALTER TABLE activity_lift_candidates
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'heuristic';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'activity_lift_candidates_source_check'
    ) THEN
        ALTER TABLE activity_lift_candidates
            ADD CONSTRAINT activity_lift_candidates_source_check
            CHECK (source IN ('heuristic', 'osm'));
    END IF;
END
$$;

ALTER TABLE activity_lift_candidates
    ADD COLUMN IF NOT EXISTS segment_effort_id UUID;

ALTER TABLE segment_efforts
    ADD COLUMN IF NOT EXISTS is_lift_segment_match BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE segment_efforts
    ADD COLUMN IF NOT EXISTS is_method_a_only BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE segment_efforts
    ADD COLUMN IF NOT EXISTS is_cable_hold_merge BOOLEAN NOT NULL DEFAULT false;

-- Partial index for the heuristic-only dedup query (fast path in
-- recomputeLiftStats: WHERE confirmed=true AND segment_effort_id IS NULL).
CREATE INDEX IF NOT EXISTS idx_alc_heuristic_only_confirmed
    ON activity_lift_candidates (activity_id)
    WHERE confirmed = true AND segment_effort_id IS NULL;
