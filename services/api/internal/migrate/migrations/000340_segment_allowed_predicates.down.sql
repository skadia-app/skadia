ALTER TABLE segments
    DROP CONSTRAINT IF EXISTS segments_allowed_predicates_valid;

ALTER TABLE segments
    DROP COLUMN IF EXISTS allowed_predicates;
