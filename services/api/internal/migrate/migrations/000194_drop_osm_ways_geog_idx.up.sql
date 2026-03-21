-- Drop unused geography GIST index on osm_ways (6.6 GB).
-- No query in the codebase uses ST_DWithin/ST_Distance on osm_ways with geography cast.
-- The geometry GIST index (osm_ways_geom_idx) is sufficient for all spatial queries.
-- ST_Length(geom::geography) for length_m calculation does not need a spatial index.
DROP INDEX IF EXISTS osm_ways_geog_idx;
