-- osm_ways.oneway_reverse column backfill.
--
-- The column is declared in services/api/internal/osm/skadia-osm.lua:57 and
-- populated by osm2pgsql on initial import (lua line 706, mapping `oneway=-1`
-- and `oneway=reverse` to true). The routing graph builder reads it via
-- COALESCE(oneway_reverse, false) in routing/graph/builder.go:309+349.
--
-- Historically only osm2pgsql's CREATE TABLE added the column, leaving DBs
-- that predate the lua schema (or were seeded differently) without it. This
-- migration makes the schema self-sufficient so fresh DBs and re-imports stop
-- failing with "column oneway_reverse does not exist".
ALTER TABLE osm_ways
    ADD COLUMN IF NOT EXISTS oneway_reverse BOOLEAN DEFAULT false;
