-- 000131_routing_cost_functions.down.sql
DROP INDEX IF EXISTS idx_osm_amenities_name_trgm;
DROP INDEX IF EXISTS idx_osm_peaks_name_trgm;
DROP FUNCTION IF EXISTS nearest_trail_node;
DROP FUNCTION IF EXISTS surface_cost_factor;
DROP FUNCTION IF EXISTS sac_cost_factor;
DROP FUNCTION IF EXISTS tobler_speed;
-- pg_trgm Extension bleibt (könnte von anderem Code benötigt werden)
