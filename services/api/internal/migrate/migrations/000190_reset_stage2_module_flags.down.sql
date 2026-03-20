-- Migration 000189 down: no-op.
-- We cannot reconstruct which rows were changed, so rollback is intentionally
-- a no-op. The stage-2 flags were all supposed to be true; this migration
-- merely restores the intended default state.
SELECT 1;
