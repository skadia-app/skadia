-- Symmetric to up.sql. osm2pgsql will re-create the column on the next
-- import via skadia-osm.lua, so dropping is safe.

ALTER TABLE osm_lifts DROP COLUMN IF EXISTS aerialway;
