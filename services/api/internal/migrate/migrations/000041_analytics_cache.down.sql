-- 000041_analytics_cache.down.sql
DROP TABLE IF EXISTS activity_rr_intervals;
DROP TABLE IF EXISTS analytics_cache;
ALTER TABLE activities DROP COLUMN IF EXISTS is_commute;
ALTER TABLE activities DROP COLUMN IF EXISTS is_race;
