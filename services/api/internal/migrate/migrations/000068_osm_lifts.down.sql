-- 000068_osm_lifts.down.sql
ALTER TABLE activities DROP COLUMN IF EXISTS exclude_lift_subtraction;
ALTER TABLE activities DROP COLUMN IF EXISTS lift_duration_s;
ALTER TABLE activities DROP COLUMN IF EXISTS lift_descent_m;
DROP TABLE IF EXISTS osm_lifts;
