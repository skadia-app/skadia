-- 000195_osm_ways_dedup_osm_id.up.sql
--
-- Deduplicate osm_ways: border ways appear in multiple regional PBF imports,
-- producing ~224k duplicate osm_ids (0.73% of 30.6M rows). The graph builder
-- used DISTINCT ON (osm_id) ORDER BY osm_id, way_id DESC which cost ~460ms
-- per tile. With a UNIQUE constraint, DISTINCT ON is no longer needed.
--
-- Strategy: keep the row with the highest way_id (most recent import).
-- osm_way_regions junction table uses osm_id (not way_id) — unaffected.

DELETE FROM osm_ways a
USING osm_ways b
WHERE a.osm_id = b.osm_id AND a.way_id < b.way_id;

-- Replace the non-unique index with a unique one.
DROP INDEX IF EXISTS idx_osm_ways_osm_id;

CREATE UNIQUE INDEX idx_osm_ways_osm_id ON osm_ways (osm_id);
