-- Drop all RLS policies first
DROP POLICY IF EXISTS user_isolation_direct ON activities;
DROP POLICY IF EXISTS user_isolation_direct ON health_hrv_snapshots;
DROP POLICY IF EXISTS user_isolation_direct ON health_sleep;
DROP POLICY IF EXISTS user_isolation_direct ON health_weight_entries;
DROP POLICY IF EXISTS user_isolation_direct ON gear;
DROP POLICY IF EXISTS user_isolation_direct ON gear_components;
DROP POLICY IF EXISTS user_isolation_direct ON pmc_daily;
DROP POLICY IF EXISTS user_isolation_direct ON user_daily_features;
DROP POLICY IF EXISTS user_isolation_direct ON tags;
DROP POLICY IF EXISTS user_isolation_direct ON segment_efforts;
DROP POLICY IF EXISTS user_isolation_direct ON activity_photos;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_streams;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_laps;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_geo_tags;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_stops;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_rr_intervals;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_tags;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_tracks;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_intervals;
DROP POLICY IF EXISTS user_isolation_via_activity ON activity_splits;

-- Disable RLS on all tables
ALTER TABLE activities DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_streams DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_laps DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_geo_tags DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_stops DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_rr_intervals DISABLE ROW LEVEL SECURITY;
ALTER TABLE health_hrv_snapshots DISABLE ROW LEVEL SECURITY;
ALTER TABLE health_sleep DISABLE ROW LEVEL SECURITY;
ALTER TABLE health_weight_entries DISABLE ROW LEVEL SECURITY;
ALTER TABLE gear DISABLE ROW LEVEL SECURITY;
ALTER TABLE gear_components DISABLE ROW LEVEL SECURITY;
ALTER TABLE pmc_daily DISABLE ROW LEVEL SECURITY;
ALTER TABLE user_daily_features DISABLE ROW LEVEL SECURITY;
ALTER TABLE tags DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_tags DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_photos DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_tracks DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_intervals DISABLE ROW LEVEL SECURITY;
ALTER TABLE activity_splits DISABLE ROW LEVEL SECURITY;
ALTER TABLE segment_efforts DISABLE ROW LEVEL SECURITY;

-- Revoke privileges and drop role
REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM skadia_readonly;
REVOKE CONNECT ON DATABASE skadia FROM skadia_readonly;
REVOKE USAGE ON SCHEMA public FROM skadia_readonly;
DO $$
BEGIN
    DROP ROLE IF EXISTS skadia_readonly;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'Could not drop skadia_readonly role: %', SQLERRM;
END
$$;
