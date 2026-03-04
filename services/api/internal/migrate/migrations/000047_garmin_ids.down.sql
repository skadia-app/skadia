DROP INDEX IF EXISTS uq_gear_garmin_id;
DROP INDEX IF EXISTS uq_activities_garmin_id;
ALTER TABLE gear DROP COLUMN IF EXISTS garmin_gear_id;
ALTER TABLE activities DROP COLUMN IF EXISTS garmin_activity_id;
