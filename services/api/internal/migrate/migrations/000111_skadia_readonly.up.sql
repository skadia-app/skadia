-- READ-ONLY role for SQL Agent (explicit whitelist)
-- Password set via env variable at deploy time
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'skadia_readonly') THEN
        CREATE ROLE skadia_readonly LOGIN PASSWORD 'skadia_readonly_password';
    END IF;
END
$$;

GRANT CONNECT ON DATABASE skadia TO skadia_readonly;
GRANT USAGE ON SCHEMA public TO skadia_readonly;
ALTER ROLE skadia_readonly SET statement_timeout = '5000';

-- Explicit table whitelist — NO grants on users, device_tokens, chat_messages
GRANT SELECT ON
    activities, activity_streams, activity_laps, activity_splits,
    activity_tracks, activity_intervals,
    activity_geo_tags, activity_stops, activity_rr_intervals, activity_photos,
    health_hrv_snapshots, health_sleep, health_weight_entries,
    gear, gear_components, segments, segment_efforts,
    pmc_daily, user_daily_features, tags, activity_tags
TO skadia_readonly;

-- Row Level Security — hard enforcement of user isolation for SQL Agent
-- RLS uses session variable set before each query: SET LOCAL app.current_user_id = '...'
-- NOTE: activity_streams is a TimescaleDB hypertable WITH columnstore compression enabled.
-- TimescaleDB does NOT support ENABLE ROW LEVEL SECURITY on compressed hypertables.
-- activity_streams is therefore excluded from RLS — SQL Agent must enforce user_id filtering manually.
ALTER TABLE activities ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_laps ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_geo_tags ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_stops ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_rr_intervals ENABLE ROW LEVEL SECURITY;
ALTER TABLE health_hrv_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE health_sleep ENABLE ROW LEVEL SECURITY;
ALTER TABLE health_weight_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE gear ENABLE ROW LEVEL SECURITY;
ALTER TABLE gear_components ENABLE ROW LEVEL SECURITY;
ALTER TABLE pmc_daily ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_daily_features ENABLE ROW LEVEL SECURITY;

-- Policy: readonly user can only see rows belonging to the current session user
CREATE POLICY user_isolation_direct ON activities FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_direct ON health_hrv_snapshots FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_direct ON health_sleep FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_direct ON health_weight_entries FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_direct ON gear FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_direct ON gear_components FOR SELECT TO skadia_readonly
    USING (gear_id IN (SELECT id FROM gear WHERE user_id = current_setting('app.current_user_id')::uuid));
CREATE POLICY user_isolation_direct ON pmc_daily FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_direct ON user_daily_features FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);

-- For tables keyed by activity_id: policy via subquery
-- NOTE: activity_streams excluded — compressed hypertable, RLS not supported
CREATE POLICY user_isolation_via_activity ON activity_laps FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));
CREATE POLICY user_isolation_via_activity ON activity_geo_tags FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));
CREATE POLICY user_isolation_via_activity ON activity_stops FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));
CREATE POLICY user_isolation_via_activity ON activity_rr_intervals FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));

-- activity_tracks und activity_intervals über activity_id
ALTER TABLE activity_tracks ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_intervals ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_isolation_via_activity ON activity_tracks FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));
CREATE POLICY user_isolation_via_activity ON activity_intervals FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));

-- Tags und Activity-Tags haben user_id → direkte RLS-Policy
ALTER TABLE tags ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_tags ENABLE ROW LEVEL SECURITY;
ALTER TABLE activity_photos ENABLE ROW LEVEL SECURITY;
ALTER TABLE segment_efforts ENABLE ROW LEVEL SECURITY;

CREATE POLICY user_isolation_direct ON tags FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_via_activity ON activity_tags FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));
CREATE POLICY user_isolation_direct ON activity_photos FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
CREATE POLICY user_isolation_direct ON segment_efforts FOR SELECT TO skadia_readonly
    USING (user_id = current_setting('app.current_user_id')::uuid);
ALTER TABLE activity_splits ENABLE ROW LEVEL SECURITY;
CREATE POLICY user_isolation_via_activity ON activity_splits FOR SELECT TO skadia_readonly
    USING (activity_id IN (SELECT id FROM activities WHERE user_id = current_setting('app.current_user_id')::uuid));
-- segments bleiben OHNE RLS (globale/öffentliche Segmente)
