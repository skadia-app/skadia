-- Revert: drop the oneway_reverse column.
-- Graph builder + lua-pipeline need the column, so downgrading requires also
-- reverting those writers.
ALTER TABLE osm_ways
    DROP COLUMN IF EXISTS oneway_reverse;
