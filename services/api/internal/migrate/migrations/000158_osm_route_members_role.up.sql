-- 000158_osm_route_members_role.up.sql
-- Add role and source columns to osm_route_way_members for Lua-based relation extraction.
-- source='relation' means extracted from osm2pgsql process_relation (exact membership),
-- source='proximity' means spatial proximity fallback (ST_DWithin).

ALTER TABLE osm_route_way_members ADD COLUMN IF NOT EXISTS role TEXT;
ALTER TABLE osm_route_way_members ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'proximity';
