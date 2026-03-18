-- osm_ways.way_id had no B-tree index since osm2pgsql recreated the table
-- without the original PRIMARY KEY from migration 000129. All enrichment
-- chunking (chunkedExec with way_id BETWEEN $1 AND $2) relied on this
-- column but did full 20GB seq scans per chunk. With 30M rows and 50k
-- chunks, this made enrichment take ~50 hours instead of ~1 hour.
CREATE INDEX IF NOT EXISTS idx_osm_ways_way_id ON osm_ways (way_id);
