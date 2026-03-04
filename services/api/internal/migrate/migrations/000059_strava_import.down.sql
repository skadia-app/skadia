DROP INDEX IF EXISTS uq_activities_strava_id;
ALTER TABLE activities DROP COLUMN IF EXISTS strava_activity_id;
DROP TABLE IF EXISTS strava_import_jobs;
