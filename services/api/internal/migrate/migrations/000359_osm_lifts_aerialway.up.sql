-- Adds the `aerialway` column to osm_lifts so backend joins for the
-- LiftCandidate.osm_aerialway field work on freshly migrated DBs (Test-DBs,
-- Production-DBs before the first osm2pgsql import).
--
-- Production-DBs that have already run osm2pgsql carry the column because
-- services/api/internal/osm/skadia-osm.lua declares it in the
-- osm2pgsql.define_way_table('osm_lifts', ...) block — but osm2pgsql only
-- creates it on import and the original 000068_osm_lifts migration omitted
-- it. ADD COLUMN IF NOT EXISTS makes the migration a no-op where the
-- column already exists.

ALTER TABLE osm_lifts ADD COLUMN IF NOT EXISTS aerialway TEXT;
